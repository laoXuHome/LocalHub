package approle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/laoXuHome/LocalHub/internal/auth"
	"github.com/laoXuHome/LocalHub/internal/buildinfo"
	"github.com/laoXuHome/LocalHub/internal/config"
	"github.com/laoXuHome/LocalHub/internal/datadir"
	"github.com/laoXuHome/LocalHub/internal/db"
	"github.com/laoXuHome/LocalHub/internal/favicon"
	"github.com/laoXuHome/LocalHub/internal/icon"
	"github.com/laoXuHome/LocalHub/internal/static"
)

type writeState struct {
	mu        sync.RWMutex
	writable  bool
	lastCheck time.Time
	lastError string
}

func (s *writeState) set(ok bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writable = ok
	s.lastCheck = time.Now()
	if err == nil {
		s.lastError = ""
	} else {
		s.lastError = err.Error()
	}
}

func (s *writeState) get() (bool, time.Time, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.writable, s.lastCheck, s.lastError
}

type server struct {
	cfg   *config.Config
	log   *slog.Logger
	db    *db.DB
	dbErr string
	state *writeState
	start time.Time
	root  string
	auth  *authState

	// P2.2 图标：本地文件存储与 Favicon 抓取。
	icons       *icon.Store
	favicon     faviconFetcher
	autoFavicon bool
}

// Run 执行 app 角色：写自检、打开数据库并迁移、提供 /health 与静态页面。
func Run(cfg *config.Config, log *slog.Logger) int {
	app := &server{cfg: cfg, log: log, state: &writeState{}, start: time.Now()}

	// 纵深防御：即使误以 root 直接启动 app，也拒绝运行（ALLOW_ROOT=1 除外）。
	if os.Geteuid() == 0 && !cfg.AllowRoot {
		log.Error("app 角色拒绝以 root 运行", slog.String("code", "ROOT_NOT_ALLOWED"), slog.String("hint", "请通过 init 角色启动，或显式设置 ALLOW_ROOT=1"))
		return 1
	}

	res, err := datadir.Resolve(cfg.DataDir, cfg.AllowedRoot)
	if err != nil {
		log.Error("数据目录校验失败", slog.String("error", err.Error()), slog.String("code", "DATA_PATH_INVALID"))
		return 1
	}
	app.root = res.Path
	log.Info("app 启动",
		slog.Int("pid", os.Getpid()),
		slog.Int("uid", os.Geteuid()),
		slog.Int("gid", os.Getegid()),
		slog.String("version", buildinfo.String()),
		slog.String("data_realpath", res.Path),
		slog.String("tz", cfg.TZ),
	)

	if err := datadir.EnsureDirs(res.Path); err != nil {
		log.Error("目录初始化失败", slog.String("error", err.Error()), slog.String("code", "DATA_DIR_CREATE_FAILED"))
		return 1
	}

	// 图标目录已由 EnsureDirs 建立；初始化失败只关闭图标能力，不影响启动。
	if st, err := icon.New(res.Path); err != nil {
		log.Warn("图标存储初始化失败，图标能力不可用", slog.String("error", err.Error()))
	} else {
		app.icons = st
		app.favicon = favicon.New()
		app.autoFavicon = true
	}

	if _, err := datadir.CheckWrite(res.Path); err != nil {
		log.Error("数据目录写自检失败",
			slog.String("error", err.Error()),
			slog.String("code", "DATA_NOT_WRITABLE"),
			slog.String("hint", fmt.Sprintf("chown -R %d:%d <宿主数据目录>", cfg.PUID, cfg.PGID)),
			slog.Bool("fatal", cfg.WriteCheckFatal),
		)
		app.state.set(false, err)
		if cfg.WriteCheckFatal {
			log.Error("写自检失败且 WRITE_CHECK_FATAL=true，退出")
			return 1
		}
		log.Warn("写自检失败但 WRITE_CHECK_FATAL=false，以 degraded 状态继续运行")
	} else {
		app.state.set(true, nil)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		// 数据库不可用不直接退出：健康检查必须以 degraded 如实反映，而不是伪装健康。
		log.Error("数据库打开失败", slog.String("error", err.Error()), slog.String("code", "DB_OPEN_FAILED"))
		app.dbErr = err.Error()
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := database.Migrate(ctx); err != nil {
			log.Error("数据库迁移失败", slog.String("error", err.Error()), slog.String("code", "DB_MIGRATE_FAILED"))
			app.dbErr = err.Error()
		} else {
			app.db = database
			log.Info("数据库就绪", slog.String("path", cfg.DBPath), slog.Int("schema_version", database.Version))
		}
		cancel()
	}
	defer func() {
		if app.db != nil {
			_ = app.db.Close()
		}
	}()

	if app.db != nil {
		app.auth = newAuthState(app)
		app.bootstrapAdmin()
		go app.sessionJanitor()
	} else {
		log.Warn("数据库不可用，认证能力将以不可用状态运行", slog.String("code", "AUTH_UNAVAILABLE"))
	}

	// 周期性写自检：挂载变化、磁盘满盘都能反映到健康状态。
	go func() {
		t := time.NewTicker(cfg.WriteCheckInterval)
		defer t.Stop()
		for range t.C {
			ok, err := func() (bool, error) {
				if _, err := datadir.CheckWrite(app.root); err != nil {
					return false, err
				}
				return true, nil
			}()
			app.state.set(ok, err)
			if !ok {
				log.Warn("周期性写自检失败", slog.String("error", err.Error()))
			}
		}
	}()

	srv := &http.Server{
		Addr:              cfg.ListenAddr(),
		Handler:           app.logMiddleware(app.routes()),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		log.Info("HTTP 服务监听", slog.String("addr", cfg.ListenAddr()))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
		close(errc)
	}()

	sigc := make(chan os.Signal, 2)
	signal.Notify(sigc, syscall.SIGTERM, syscall.SIGINT)
	select {
	case sig := <-sigc:
		log.Info("收到信号，开始优雅关闭", slog.String("signal", sig.String()))
	case err := <-errc:
		if err != nil {
			log.Error("HTTP 服务异常退出", slog.String("error", err.Error()))
			return 1
		}
		return 0
	}

	shCtx, shCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shCancel()
	if err := srv.Shutdown(shCtx); err != nil {
		log.Warn("优雅关闭超时", slog.String("error", err.Error()))
		return 1
	}
	log.Info("HTTP 服务已关闭")
	return 0
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	dbStatus := "ok"
	dbErr := s.dbErr
	dbVersion := 0
	if s.db != nil {
		dbVersion = s.db.Version
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if s.db == nil {
		dbStatus = "error"
		if dbErr == "" {
			dbErr = "数据库未初始化"
		}
	} else if err := s.db.Ping(ctx); err != nil {
		dbStatus = "error"
		dbErr = err.Error()
	}
	writable, lastCheck, writeErr := s.state.get()

	status := "ok"
	if dbStatus != "ok" || !writable {
		status = "degraded"
	}

	last := ""
	if !lastCheck.IsZero() {
		last = lastCheck.Format(time.RFC3339)
	}

	resp := map[string]any{
		"status":         status,
		"version":        buildinfo.Version,
		"commit":         buildinfo.Commit,
		"build_time":     buildinfo.BuildTime,
		"uptime_seconds": int(time.Since(s.start).Seconds()),
		"tz":             s.cfg.TZ,
		"database": map[string]any{
			"status":         dbStatus,
			"path":           s.cfg.DBPath,
			"schema_version": dbVersion,
			"error":          dbErr,
		},
		"data": map[string]any{
			"path":         s.root,
			"allowed_root": s.cfg.AllowedRoot,
			"writable":     writable,
			"last_check":   last,
			"error":        writeErr,
		},
		"process": map[string]any{
			"role":    "app",
			"pid":     os.Getpid(),
			"uid":     os.Geteuid(),
			"gid":     os.Getegid(),
			"is_root": os.Geteuid() == 0,
		},
	}

	code := http.StatusOK
	if status != "ok" {
		code = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(resp)
}

// routes 组装路由：公开接口 + 管理员专属接口（后端强制鉴权）。
func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)

	// 公开：登录状态查询、登录。
	mux.HandleFunc("/api/auth/status", s.handleAuthStatus)
	mux.HandleFunc("/api/auth/login", s.handleLogin)

	// 管理员专属：退出、改密、管理员信息。
	mux.Handle("/api/auth/logout", s.requireAdmin(http.HandlerFunc(s.handleLogout)))
	mux.Handle("/api/auth/password", s.requireAdmin(http.HandlerFunc(s.handleChangePassword)))
	mux.Handle("GET /api/admin/whoami", s.requireAdmin(http.HandlerFunc(s.handleAdminWhoami)))

	// 书签：公开读取按权限过滤；管理操作全部走后端鉴权。
	mux.HandleFunc("GET /api/bookmarks", s.handlePublicBookmarks)
	mux.HandleFunc("GET /api/bookmarks/{id}", s.handlePublicBookmark)
	mux.Handle("GET /api/admin/bookmarks", s.requireAdmin(http.HandlerFunc(s.handleAdminListBookmarks)))
	mux.Handle("POST /api/admin/bookmarks", s.requireAdmin(http.HandlerFunc(s.handleAdminCreateBookmark)))
	mux.Handle("POST /api/admin/bookmarks/batch-delete", s.requireAdmin(http.HandlerFunc(s.handleAdminBatchDelete)))
	mux.Handle("POST /api/admin/bookmarks/order", s.requireAdmin(http.HandlerFunc(s.handleAdminReorder)))
	mux.Handle("GET /api/admin/bookmarks/{id}", s.requireAdmin(http.HandlerFunc(s.handleAdminGetBookmark)))
	mux.Handle("PUT /api/admin/bookmarks/{id}", s.requireAdmin(http.HandlerFunc(s.handleAdminUpdateBookmark)))
	mux.Handle("DELETE /api/admin/bookmarks/{id}", s.requireAdmin(http.HandlerFunc(s.handleAdminDeleteBookmark)))
	mux.Handle("POST /api/admin/bookmarks/import/preview", s.requireAdmin(http.HandlerFunc(s.handleImportPreview)))
	mux.Handle("POST /api/admin/bookmarks/import/execute", s.requireAdmin(http.HandlerFunc(s.handleImportExecute)))
	mux.Handle("GET /api/admin/bookmarks/export", s.requireAdmin(http.HandlerFunc(s.handleExport)))

	// P2.2 图标：本地文件读取与管理员管理。
	mux.HandleFunc("GET /api/bookmarks/{id}/icon", s.handleBookmarkIcon)
	mux.Handle("PUT /api/admin/bookmarks/{id}/icon", s.requireAdmin(http.HandlerFunc(s.handleBookmarkIconUpload)))
	mux.Handle("DELETE /api/admin/bookmarks/{id}/icon", s.requireAdmin(http.HandlerFunc(s.handleBookmarkIconDelete)))
	mux.Handle("POST /api/admin/bookmarks/{id}/favicon", s.requireAdmin(http.HandlerFunc(s.handleBookmarkFavicon)))

	// 其余为公开静态页面（访客可访问），/api/ 下未定义的路径由静态处理器返回 404。
	mux.Handle("/", s.staticHandler())
	return mux
}

func (s *server) bootstrapAdmin() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	n, err := s.auth.store.CountAdmins(ctx)
	if err != nil {
		s.log.Error("检查管理员初始化状态失败", slog.String("error", err.Error()), slog.String("code", "ADMIN_CHECK_FAILED"))
		return
	}
	if n > 0 {
		if s.cfg.AdminPassword != "" {
			s.log.Warn("管理员已存在，忽略 LOCALHUB_ADMIN_PASSWORD（不会覆盖已有账户）")
		}
		return
	}
	if s.cfg.AdminPassword == "" {
		s.log.Warn("管理员尚未初始化",
			slog.String("code", "ADMIN_NOT_INITIALIZED"),
			slog.String("hint", "设置 LOCALHUB_ADMIN_PASSWORD（12-72 字节）后重启容器完成首次初始化；已初始化后该变量可移除"),
		)
		return
	}
	hash, err := auth.HashPassword(s.cfg.AdminPassword)
	if err != nil {
		s.log.Error("初始口令不符合策略，管理员未创建", slog.String("error", err.Error()), slog.String("code", "ADMIN_PASSWORD_INVALID"))
		return
	}
	if err := s.auth.store.CreateAdmin(ctx, s.cfg.AdminUsername, hash, time.Now()); err != nil {
		s.log.Error("管理员初始化失败", slog.String("error", err.Error()), slog.String("code", "ADMIN_INIT_FAILED"))
		return
	}
	s.log.Info("管理员初始化完成", slog.String("username", s.cfg.AdminUsername), slog.String("code", "ADMIN_INITIALIZED"))
}

// sessionJanitor 定时清理过期会话，避免会话表无限增长。
func (s *server) sessionJanitor() {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for range t.C {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		n, err := s.auth.sessions.PurgeExpired(ctx, time.Now())
		cancel()
		if err != nil {
			s.log.Warn("清理过期会话失败", slog.String("error", err.Error()))
		} else if n > 0 {
			s.log.Info("清理过期会话", slog.Int64("count", n))
		}
	}
}

func (s *server) staticHandler() http.Handler {
	sub, err := fs.Sub(static.FS, "dist")
	if err != nil {
		s.log.Error("静态资源加载失败", slog.String("error", err.Error()))
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "static assets unavailable", http.StatusInternalServerError)
		})
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		// SPA 回退：非静态文件路径统一返回 index.html（P0 只有占位页）。
		if _, err := fs.Stat(sub, strings.TrimPrefix(r.URL.Path, "/")); err != nil && r.URL.Path != "/" {
			r.URL.Path = "/"
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		fileServer.ServeHTTP(w, r)
	})
}

func (s *server) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		s.log.Info("http 请求",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", sw.status),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
