package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/laoXuHome/LocalHub/internal/approle"
	"github.com/laoXuHome/LocalHub/internal/buildinfo"
	"github.com/laoXuHome/LocalHub/internal/config"
	"github.com/laoXuHome/LocalHub/internal/datadir"
	"github.com/laoXuHome/LocalHub/internal/initrole"
	"github.com/laoXuHome/LocalHub/internal/logging"
)

func main() {
	role := flag.String("role", "init", "运行角色：init（默认，PID1 入口）或 app（业务进程）")
	selftest := flag.Bool("selftest", false, "以当前调用者身份验证 /data 可写性并输出 JSON")
	showVersion := flag.Bool("version", false, "输出版本信息")
	healthcheck := flag.Bool("healthcheck", false, "容器健康检查：请求本地 /health，成功返回 0")
	flag.Parse()

	if *showVersion {
		fmt.Printf("localhub %s commit=%s build_time=%s\n", buildinfo.Version, buildinfo.Commit, buildinfo.BuildTime)
		return
	}

	if *healthcheck {
		os.Exit(runHealthcheck())
	}

	if *selftest {
		os.Exit(runSelftest())
	}

	cfg, err := config.Load()
	if err != nil {
		// 配置错误必须响亮失败，不能静默忽略。
		fmt.Fprintf(os.Stderr, `{"level":"ERROR","msg":"配置校验失败","error":%q}`+"\n", err.Error())
		os.Exit(1)
	}

	log := logging.New(cfg.LogLevel, *role)
	switch *role {
	case "init":
		self, err := os.Executable()
		if err != nil {
			log.Error("无法定位自身可执行文件", "error", err.Error())
			os.Exit(1)
		}
		os.Exit(initrole.Run(cfg, log, self))
	case "app":
		os.Exit(approle.Run(cfg, log))
	default:
		fmt.Fprintf(os.Stderr, "未知角色：%s（可选 init / app）\n", *role)
		os.Exit(2)
	}
}

type selftestReport struct {
	DataWritable  bool   `json:"data_writable"`
	Verified      bool   `json:"verified"`
	UID           int    `json:"uid"`
	GID           int    `json:"gid"`
	IsRoot        bool   `json:"is_root"`
	DataDir       string `json:"data_dir"`
	DataRealpath  string `json:"data_realpath"`
	AllowedRoot   string `json:"allowed_root"`
	AllowedRootOK bool   `json:"allowed_root_ok"`
	ExpectedUID   int    `json:"expected_puid"`
	ExpectedGID   int    `json:"expected_pgid"`
	Reason        string `json:"reason"`
	Version       string `json:"version"`
	Commit        string `json:"commit"`
}

// runSelftest 在“调用者自身权限”下验证写入能力。
// 特别注意：以 root 运行不代表业务进程（PUID/PGID）具备写权限，因此 root 且业务 UID 不同时判定为未通过。
func runSelftest() int {
	cfg, err := config.Load()
	if err != nil {
		writeJSON(selftestReport{Reason: "配置校验失败：" + err.Error(), Version: buildinfo.Version, Commit: buildinfo.Commit})
		return 1
	}

	rep := selftestReport{
		UID:         os.Geteuid(),
		GID:         os.Getegid(),
		IsRoot:      os.Geteuid() == 0,
		DataDir:     cfg.DataDir,
		AllowedRoot: cfg.AllowedRoot,
		ExpectedUID: cfg.PUID,
		ExpectedGID: cfg.PGID,
		Version:     buildinfo.Version,
		Commit:      buildinfo.Commit,
	}

	res, err := datadir.Resolve(cfg.DataDir, cfg.AllowedRoot)
	if err != nil {
		rep.Reason = "路径校验失败：" + err.Error()
		writeJSON(rep)
		return 1
	}
	rep.DataRealpath = res.Path
	rep.AllowedRootOK = true

	if _, err := datadir.CheckWrite(res.Path); err != nil {
		rep.DataWritable = false
		rep.Reason = "写入失败：" + err.Error()
		writeJSON(rep)
		return 1
	}
	rep.DataWritable = true

	if rep.IsRoot && cfg.PUID != 0 && !cfg.AllowRoot {
		rep.Verified = false
		rep.Reason = fmt.Sprintf("以 root 运行，无法证明业务进程（PUID=%d/PGID=%d）的写权限；请用 docker exec --user %d:%d 执行本命令", cfg.PUID, cfg.PGID, cfg.PUID, cfg.PGID)
		writeJSON(rep)
		return 1
	}

	rep.Verified = true
	writeJSON(rep)
	return 0
}

// runHealthcheck 供容器 HEALTHCHECK 使用（distroless 无 curl/sh）。
func runHealthcheck() int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "配置校验失败："+err.Error())
		return 1
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", cfg.HTTPPort))
	if err != nil {
		fmt.Fprintln(os.Stderr, "健康检查请求失败："+err.Error())
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "健康检查失败：HTTP %d\n", resp.StatusCode)
		return 1
	}
	return 0
}

func writeJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
