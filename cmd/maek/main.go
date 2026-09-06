package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gosuda/maek/internal/agent"
	"github.com/gosuda/maek/internal/server"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	subcommand := os.Args[1]

	switch subcommand {
	case "server", "serve":
		runServer(os.Args[2:])
	case "agent", "connect":
		runAgent(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Printf("maek version %s (commit: %s, date: %s)\n", version, commit, date)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", subcommand)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`maek - ngrok-like HTTP reverse proxy tunnel via WebSocket & Yamux (v%s)

Usage:
  maek server [flags]    Run the central maek server on a public VPS
  maek agent [flags]     Run the local agent to tunnel a private service
  maek version           Show version

Server Flags:
  -addr, -p string       Listen address (default ":8080")

Agent Flags:
  -server, -s string     Central maek server URL (e.g. "ws://vps-ip:8080")
  -name, -n string       Service identifier name (optional, auto-detected from target)
  -id, -i string         Preferred custom service ID (max 32 chars, optional)
  -desc, -d string       Short description of the service (optional, auto-detected)
  -thumb string          Thumbnail URL or image avatar (optional, auto-detected)
  -target, -t string     Local target HTTP URL (default "http://localhost:3000")

Examples:
  # Start central server
  maek server -p 8080

  # Zero-config tunnel: auto-detects name, description, and icon from target
  maek agent -s ws://1.2.3.4:8080 -t http://localhost:3000

  # Explicit naming and custom ID
  maek agent -s ws://1.2.3.4:8080 -n dev-app -i my-dev -t http://localhost:3000
`, version)
}

func runServer(args []string) {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "Address to listen on")
	pShort := fs.String("p", "", "Address to listen on (short)")

	if err := fs.Parse(args); err != nil {
		log.Fatalf("Invalid flags: %v", err)
	}

	listenAddr := *addr
	if *pShort != "" {
		listenAddr = *pShort
	}
	if !strings.Contains(listenAddr, ":") {
		listenAddr = ":" + listenAddr
	}

	srv := server.NewServer(server.Config{
		Addr: listenAddr,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := srv.Start(); err != nil && err.Error() != "http: Server closed" {
			log.Fatalf("[maek-server] Server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("[maek-server] Shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

func runAgent(args []string) {
	fs := flag.NewFlagSet("agent", flag.ExitOnError)
	serverURL := fs.String("server", "", "maek server URL (e.g. ws://vps-ip:8080)")
	sShort := fs.String("s", "", "maek server URL (short)")
	name := fs.String("name", "", "Service name (optional, auto-detected from target)")
	nShort := fs.String("n", "", "Service name (short)")
	prefID := fs.String("id", "", "Preferred service ID (max 32 chars, optional)")
	iShort := fs.String("i", "", "Preferred service ID (short)")
	desc := fs.String("desc", "", "Short description of the service (optional)")
	dShort := fs.String("d", "", "Short description of the service (short)")
	thumb := fs.String("thumb", "", "Thumbnail URL or image avatar (optional)")
	target := fs.String("target", "http://localhost:3000", "Local target URL (kept private to agent)")
	tShort := fs.String("t", "", "Local target URL (short)")

	if err := fs.Parse(args); err != nil {
		log.Fatalf("Invalid flags: %v", err)
	}

	srvURL := *serverURL
	if *sShort != "" {
		srvURL = *sShort
	}
	if srvURL == "" {
		fmt.Fprintln(os.Stderr, "Error: -server (or -s) flag is required.")
		fs.Usage()
		os.Exit(1)
	}

	svcName := *name
	if *nShort != "" {
		svcName = *nShort
	}

	preferredID := *prefID
	if *iShort != "" {
		preferredID = *iShort
	}
	if preferredID == "" && svcName != "" {
		preferredID = svcName
	}

	svcDesc := *desc
	if *dShort != "" {
		svcDesc = *dShort
	}

	targetURL := *target
	if *tShort != "" {
		targetURL = *tShort
	}

	ag, err := agent.NewAgent(agent.Config{
		ServerURL:   srvURL,
		Name:        svcName,
		PreferredID: preferredID,
		Description: svcDesc,
		Thumbnail:   *thumb,
		Target:      targetURL,
	})
	if err != nil {
		log.Fatalf("[maek-agent] Initialization failed: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("[maek-agent] Starting agent for '%s' -> %s", ag.Config().Name, targetURL)
	if err := ag.Start(ctx); err != nil && err != context.Canceled {
		log.Fatalf("[maek-agent] Exited: %v", err)
	}
}
