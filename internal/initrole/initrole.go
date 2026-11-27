package initrole

import (
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/laoXuHome/LocalHub/internal/config"
	"github.com/laoXuHome/LocalHub/internal/datadir"
)

const shutdownTimeout = 15 * time.Second

// Run 执行 init（PID1）角色：校验参数、修正 /data 属主、以非 root 启动业务进程、转发信号、透传退出码。
func Run(cfg *config.Config, log *slog.Logger, self string) int {
	euid := os.Geteuid()
	log.Info("init 启动",
		slog.Int("pid", os.Getpid()),
		slog.Int("euid", euid),
		slog.Int("egid", os.Getegid()),
		slog.String("data_dir", cfg.DataDir),
		slog.String("allowed_root", cfg.AllowedRoot),
		slog.Int("puid", cfg.PUID),
		slog.Int("pgid", cfg.PGID),
		slog.Bool("allow_root", cfg.AllowRoot),
	)

	if cfg.PUID == 0 && !cfg.AllowRoot {
		log.Error("PUID=0 被拒绝：业务进程不得以 root 运行（如需此例外必须显式设置 ALLOW_ROOT=1）")
		return 1
	}
	if cfg.PUID == 0 && cfg.AllowRoot {
		log.Warn("ALLOW_ROOT=1 已启用：业务进程将以 root 运行，非 root 保证失效，仅建议用于排障")
	}

	res, err := datadir.Resolve(cfg.DataDir, cfg.AllowedRoot)
	if err != nil {
		log.Error("数据目录校验失败", slog.String("error", err.Error()), slog.String("code", "DATA_PATH_INVALID"))
		return 1
	}
	log.Info("数据目录校验通过", slog.String("realpath", res.Path), slog.String("allowed_root", res.AllowedRoot))

	if err := datadir.EnsureDirs(res.Path); err != nil {
		log.Error("目录初始化失败", slog.String("error", err.Error()), slog.String("code", "DATA_DIR_CREATE_FAILED"))
		if euid == 0 {
			log.Error("提示：容器需要 CAP_CHOWN/CAP_SETUID/CAP_SETGID，请勿使用 cap_drop: ALL；并确认挂载目录可写")
		}
		return 1
	}

	if euid == 0 {
		if err := datadir.FixOwnership(res.Path, cfg.PUID, cfg.PGID, cfg.ChownRecursive); err != nil {
			log.Error("属主修正失败",
				slog.String("error", err.Error()),
				slog.String("code", "CHOWN_FAILED"),
				slog.String("hint", "需要 CAP_CHOWN；如已 cap_drop: ALL，请保留 CHOWN/SETUID/SETGID 或改用匹配属主挂载"),
			)
			return 1
		}
		log.Info("属主修正完成", slog.Int("uid", cfg.PUID), slog.Int("gid", cfg.PGID), slog.Bool("recursive", cfg.ChownRecursive))
	} else if euid != cfg.PUID {
		log.Error("init 以非 root 运行，且当前 UID 与 PUID 不一致",
			slog.Int("euid", euid), slog.Int("puid", cfg.PUID),
			slog.String("code", "UID_MISMATCH"),
			slog.String("hint", "以 root 启动以便修正属主，或用 --user 指定与 PUID/PGID 一致的 UID/GID"),
		)
		return 1
	}

	cmd := exec.Command(self, "--role=app")
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if euid == 0 && cfg.PUID != 0 {
		// 不在多线程进程中调用 setuid；由父进程在 fork 时指定子进程凭据。
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{
				Uid:    uint32(cfg.PUID),
				Gid:    uint32(cfg.PGID),
				Groups: []uint32{uint32(cfg.PGID)},
			},
		}
	}

	log.Info("启动业务进程", slog.String("exec", self), slog.String("args", "--role=app"), slog.Int("puid", cfg.PUID), slog.Int("pgid", cfg.PGID))
	if err := cmd.Start(); err != nil {
		code := "APP_START_FAILED"
		hint := "检查 PUID/PGID 与容器能力"
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			code = "PRIVILEGE_DENIED"
			hint = "降权失败：切换到 PUID/PGID 需要 CAP_SETUID 与 CAP_SETGID，两者缺一不可；请勿使用 cap_drop: ALL"
		}
		log.Error("业务进程启动失败", slog.String("error", err.Error()), slog.String("code", code), slog.String("hint", hint))
		return 1
	}
	appPID := cmd.Process.Pid
	log.Info("业务进程已启动", slog.Int("app_pid", appPID), slog.Int("puid", cfg.PUID), slog.Int("pgid", cfg.PGID))

	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	for {
		select {
		case sig := <-sigs:
			log.Info("收到信号，转发给业务进程", slog.String("signal", sig.String()), slog.Int("app_pid", appPID))
			if err := cmd.Process.Signal(sig); err != nil {
				log.Warn("转发信号失败", slog.String("error", err.Error()))
			}
			timer := time.NewTimer(shutdownTimeout)
			select {
			case werr := <-done:
				timer.Stop()
				return exitCode(werr, log)
			case <-timer.C:
				log.Warn("业务进程未在超时内退出，发送 SIGKILL", slog.Duration("timeout", shutdownTimeout))
				_ = cmd.Process.Kill()
				return exitCode(<-done, log)
			}
		case werr := <-done:
			return exitCode(werr, log)
		}
	}
}

func exitCode(werr error, log *slog.Logger) int {
	if werr == nil {
		log.Info("业务进程正常退出", slog.Int("exit_code", 0))
		return 0
	}
	var ee *exec.ExitError
	if errors.As(werr, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			code := 128 + int(ws.Signal())
			log.Warn("业务进程被信号终止", slog.Int("signal", int(ws.Signal())), slog.Int("exit_code", code))
			return code
		}
		log.Warn("业务进程异常退出", slog.Int("exit_code", ee.ExitCode()))
		return ee.ExitCode()
	}
	log.Error("等待业务进程失败", slog.String("error", werr.Error()))
	return 1
}
