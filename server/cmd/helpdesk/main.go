// Сервер техподдержки.
//
//	helpdesk serve         — запустить API
//	helpdesk create-user   — завести пользователя (см. -h)
//	helpdesk sync-effcon   — синхронизировать пользователей из «Оценки эффективности»
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"spsatech/helpdesk/internal/api"
	"spsatech/helpdesk/internal/auth"
	"spsatech/helpdesk/internal/config"
	"spsatech/helpdesk/internal/db"
	"spsatech/helpdesk/internal/download"
	"spsatech/helpdesk/internal/effcon"
	"spsatech/helpdesk/internal/files"
	"spsatech/helpdesk/internal/panel"
	"spsatech/helpdesk/internal/push"
	"spsatech/helpdesk/internal/store"
)

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve()
	case "create-user":
		err = createUser(os.Args[2:])
	case "sync-effcon":
		err = syncEffcon()
	default:
		err = fmt.Errorf("unknown command %q (expected serve or create-user)", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func serve() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}
	storage, err := files.NewStorage(cfg.UploadDir)
	if err != nil {
		return err
	}

	st := store.New(pool)
	tokens := auth.NewTokens(cfg.JWTSecret, cfg.TokenTTL)
	limiter := auth.NewLoginLimiter()
	var fcm *push.FCM
	if cfg.FCMCredentialsFile != "" {
		key, err := os.ReadFile(cfg.FCMCredentialsFile)
		if err != nil {
			return fmt.Errorf("FCM_CREDENTIALS_FILE: %w", err)
		}
		if fcm, err = push.NewFCM(ctx, key); err != nil {
			return err
		}
		slog.Info("push notifications enabled")
	} else {
		slog.Warn("FCM_CREDENTIALS_FILE not set: push notifications disabled")
	}
	push.NewWorker(st, fcm).Start(ctx)

	var sched *effcon.Scheduler
	if cfg.EffconURL != "" {
		sched = effcon.NewScheduler(pool, cfg.EffconURL)
		sched.Start(ctx)
	}
	pnl, err := panel.New(st, tokens, storage, limiter, cfg.JWTSecret, cfg.TokenTTL, pool, sched)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.New(st, tokens, storage, limiter).Routes(pnl.Register, download.New(cfg.APKDir).Register),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	slog.Info("listening", "addr", cfg.ListenAddr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func createUser(args []string) error {
	fl := flag.NewFlagSet("create-user", flag.ExitOnError)
	login := fl.String("login", "", "логин (обязательно)")
	name := fl.String("name", "", "ФИО (обязательно)")
	role := fl.String("role", "employee", "роль: employee, support, admin")
	dept := fl.String("department", "", "подразделение")
	phone := fl.String("phone", "", "телефон")
	temp := fl.Bool("temporary", false, "потребовать смену пароля при первом входе")
	fl.Parse(args)

	if *login == "" || *name == "" {
		fl.Usage()
		return errors.New("-login and -name are required")
	}
	if !store.Role(*role).Valid() {
		return fmt.Errorf("invalid role %q", *role)
	}

	// Пароль читаем из stdin, чтобы он не попадал в историю команд.
	fmt.Fprint(os.Stderr, "Пароль: ")
	password, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && password == "" {
		return fmt.Errorf("read password: %w", err)
	}
	password = strings.TrimRight(password, "\r\n")
	if len([]rune(password)) < auth.MinPasswordLength {
		return fmt.Errorf("password must be at least %d characters", auth.MinPasswordLength)
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	u, err := store.New(pool).CreateUser(ctx, store.NewUser{
		Login: strings.ToLower(strings.TrimSpace(*login)), PasswordHash: hash, FullName: *name,
		Department: *dept, Phone: *phone, Role: store.Role(*role), MustChangePassword: *temp,
	})
	if errors.Is(err, store.ErrDuplicate) {
		return fmt.Errorf("login %q already exists", *login)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "\nСоздан пользователь #%d %s (%s)\n", u.ID, u.Login, u.Role)
	return nil
}

func syncEffcon() error {
	dbURL, srcURL := os.Getenv("DATABASE_URL"), os.Getenv("EFFCON_DATABASE_URL")
	if dbURL == "" || srcURL == "" {
		return errors.New("DATABASE_URL and EFFCON_DATABASE_URL are required")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}
	rep, err := effcon.NewScheduler(pool, srcURL).RunNow(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("В effcon: %d, действующих: %d, без должности: %d\n", rep.SourceTotal, rep.Eligible, rep.NoPosition)
	fmt.Printf("Создано: %d, обновлено: %d, без изменений: %d, заблокировано: %d\n",
		rep.Created, rep.Updated, rep.Unchanged, rep.Deactivated)
	for _, s := range rep.Skipped {
		fmt.Printf("Пропущен %s (%s): %s\n", s.Login, s.Name, s.Reason)
	}
	return nil
}
