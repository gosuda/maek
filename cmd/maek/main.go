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

	"github.com/gosuda/maek/internal/agent"
	"github.com/gosuda/maek/internal/server"
	"github.com/gosuda/maek/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "server", "serve":
		runServer(os.Args[2:])
	case "agent", "connect":
		runAgent(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Println(version.String())
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
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
  -alias, -a string      Human-readable service alias (optional, auto-detected)
  -id, -i string         Preferred custom service ID (max 32 chars, optional)
  -desc, -d string       Short description (optional, auto-detected)
  -thumb string          Thumbnail URL or image avatar (optional, auto-detected)
  -target, -t string     Local target HTTP URL (default "http://localhost:3000")

Examples:
  maek server -p 8080
  maek agent -s ws://1.2.3.4:8080 -t http://localhost:3000
  maek agent -s ws://1.2.3.4:8080 -a dev-app -i my-dev -t http://localhost:3000
`, version.Version)
}

func runServer(args []string) {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "Address to listen on")
	pShort := fs.String("p", "", "Address to listen on (short)")
	_ = fs.Parse(args)
	listenAddr := *addr
	if *pShort != "" {
		listenAddr = *pShort
	}
	if !strings.Contains(listenAddr, ":") {
		listenAddr = ":" + listenAddr
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := server.NewServer(server.Config{Addr: listenAddr}).Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("[maek-server] exited: %v", err)
	}
}

func runAgent(args []string) {
	fs := flag.NewFlagSet("agent", flag.ExitOnError)
	serverURL := fs.String("server", "", "maek server URL")
	sShort := fs.String("s", "", "maek server URL (short)")
	alias := fs.String("alias", "", "Service alias (optional, auto-detected)")
	aShort := fs.String("a", "", "Service alias (short)")
	prefID := fs.String("id", "", "Preferred service ID")
	iShort := fs.String("i", "", "Preferred service ID (short)")
	desc := fs.String("desc", "", "Short description")
	dShort := fs.String("d", "", "Short description (short)")
	thumb := fs.String("thumb", "", "Thumbnail URL or image avatar")
	target := fs.String("target", "http://localhost:3000", "Local target URL")
	tShort := fs.String("t", "", "Local target URL (short)")
	_ = fs.Parse(args)

	srvURL := *serverURL
	if *sShort != "" {
		srvURL = *sShort
	}
	if srvURL == "" {
		fmt.Fprintln(os.Stderr, "Error: -server (or -s) flag is required.")
		os.Exit(1)
	}
	serviceAlias := *alias
	if *aShort != "" {
		serviceAlias = *aShort
	}
	preferredID := *prefID
	if *iShort != "" {
		preferredID = *iShort
	}
	serviceDesc := *desc
	if *dShort != "" {
		serviceDesc = *dShort
	}
	targetURL := *target
	if *tShort != "" {
		targetURL = *tShort
	}

	ag, err := agent.NewAgent(agent.Config{
		ServerURL: srvURL,
		Services: []agent.ServiceConfig{{
			Alias:       serviceAlias,
			PreferredID: preferredID,
			Description: serviceDesc,
			Thumbnail:   *thumb,
			Target:      targetURL,
		}},
	})
	if err != nil {
		log.Fatalf("[maek-agent] initialization failed: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := ag.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("[maek-agent] exited: %v", err)
	}
}
