package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mojro/docs-platform/internal/builder"
	"github.com/mojro/docs-platform/internal/handler"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		log.Printf("ignoring invalid %s=%q, using %d", key, v, def)
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
		log.Printf("ignoring invalid %s=%q, using %v", key, v, def)
	}
	return def
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func main() {
	dataDir := env("DATA_DIR", "/data")
	addr := env("LISTEN_ADDR", ":8080")
	secret := os.Getenv("WEBHOOK_SECRET")
	if secret == "" {
		log.Fatal("WEBHOOK_SECRET is required")
	}

	siteDir := filepath.Join(dataDir, "site")
	protectedRefs := splitList(env("PROTECTED_REFS", "main,master,develop"))
	maxVersions := envInt("MAX_VERSIONS", 5)
	publishFeature := envBool("PUBLISH_FEATURE_BRANCHES", true)

	b := builder.New(builder.Config{
		ReposDir:      filepath.Join(dataDir, "repos"),
		TreeDir:       filepath.Join(dataDir, "tree"),
		SiteDir:       siteDir,
		MetaDir:       filepath.Join(dataDir, "meta"),
		BaseConfig:    env("MKDOCS_BASE_CONFIG", "/app/mkdocs-base.yml"),
		CommonIndex:   env("COMMON_INDEX", "/app/docs/index.md"),
		RepoWorkspace: env("REPO_WORKSPACE", "mojro"),
		ProtectedRefs: protectedRefs,
		MaxVersions:   maxVersions,
	})

	h := handler.New(b, handler.Config{
		Secret:                 secret,
		SiteDir:                siteDir,
		PublishFeatureBranches: publishFeature,
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           h.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("docs-platform listening on %s", addr)
		log.Printf("protected refs: %v (always published, never evicted)", protectedRefs)
		log.Printf("feature branches: publish=%v, keeping newest %d", publishFeature, maxVersions)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
