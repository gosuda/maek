package server

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/gosuda/maek/internal/protocol"
	"github.com/gosuda/maek/internal/service"
	"github.com/gosuda/maek/internal/tunnel"
	"github.com/gosuda/maek/internal/version"
)

func (s *Server) handleAgentWebSocket(w http.ResponseWriter, r *http.Request) {
	wsConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols:       protocol.SupportedSubprotocols(),
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.Printf("[maek-server] websocket handshake failed: %v", err)
		return
	}
	defer wsConn.CloseNow()

	hsCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	fail := func(code, message string) {
		_ = tunnel.WriteControl(context.Background(), wsConn, protocol.MsgErrorV1, protocol.ProtocolError{Code: code, Message: message})
		_ = wsConn.Close(websocket.StatusPolicyViolation, code)
	}

	config, err := tunnel.ServerNegotiate(hsCtx, wsConn, version.Version)
	if err != nil {
		fail("negotiation-failed", err.Error())
		return
	}

	env, err := tunnel.ReadControl(hsCtx, wsConn)
	if err != nil || env.Type != protocol.MsgRegisterV1 {
		fail("invalid-register", "expected a register message")
		return
	}
	request, err := protocol.DecodePayload[protocol.RegisterServices](env)
	if err != nil || len(request.Services) == 0 {
		fail("invalid-register", "at least one service is required")
		return
	}
	if len(request.Services) > 64 {
		fail("too-many-services", "a session may register at most 64 services")
		return
	}

	type pendingService struct {
		reservation *Reservation
		spec        protocol.ServiceSpec
		assigned    protocol.AssignedService
	}
	pending := make([]pendingService, 0, len(request.Services))
	for i, spec := range request.Services {
		alias := strings.TrimSpace(spec.Alias)
		if alias == "" {
			alias = "app"
		}
		preferredID := strings.TrimSpace(spec.PreferredID)
		if preferredID == "" {
			preferredID = alias
		}
		reservation, err := s.registry.ReserveID(preferredID)
		if err != nil {
			for _, item := range pending {
				item.reservation.Release()
			}
			fail("id-allocation", "failed to reserve service ID")
			return
		}
		pending = append(pending, pendingService{
			reservation: reservation,
			spec:        spec,
			assigned: protocol.AssignedService{
				Index: uint16(i),
				ID:    reservation.ID(),
				Alias: alias,
			},
		})
	}
	defer func() {
		for _, item := range pending {
			item.reservation.Release()
		}
	}()

	assigned := make([]protocol.AssignedService, 0, len(pending))
	for _, item := range pending {
		assigned = append(assigned, item.assigned)
	}
	if err := tunnel.WriteControl(hsCtx, wsConn, protocol.MsgRegistered, protocol.RegisteredServices{Services: assigned}); err != nil {
		return
	}
	env, err = tunnel.ReadControl(hsCtx, wsConn)
	if err != nil || env.Type != protocol.MsgStartV1 {
		fail("expected-start", "expected a start message")
		return
	}

	netConn := websocket.NetConn(context.Background(), wsConn, websocket.MessageBinary)
	tunnelSession, err := tunnel.NewServer(netConn, config)
	if err != nil {
		log.Printf("[maek-server] failed to create tunnel session: %v", err)
		return
	}
	defer tunnelSession.Close()

	connectedAt := time.Now()
	registrations := make([]*Registration, 0, len(pending))
	for i, item := range pending {
		info := service.Info{
			ID:          item.assigned.ID,
			Alias:       item.assigned.Alias,
			Description: strings.TrimSpace(item.spec.Description),
			Thumbnail:   strings.TrimSpace(item.spec.Thumbnail),
			ConnectedAt: connectedAt.Add(time.Duration(i) * time.Nanosecond),
			RemoteAddr:  r.RemoteAddr,
		}
		proxy := BuildTunnelProxy(tunnelSession, info.ID, info.Alias)
		_, registration, err := s.registry.Activate(item.reservation, info, proxy)
		if err != nil {
			for _, active := range registrations {
				active.Close()
			}
			log.Printf("[maek-server] failed to activate service %s: %v", info.ID, err)
			return
		}
		registrations = append(registrations, registration)
		log.Printf("[maek-server] registered alias %q (ID: %s) from %s", info.Alias, info.ID, r.RemoteAddr)
	}
	defer func() {
		for _, registration := range registrations {
			registration.Close()
		}
	}()

	<-tunnelSession.Done()
	for _, item := range pending {
		log.Printf("[maek-server] disconnected alias %q (ID: %s)", item.assigned.Alias, item.assigned.ID)
	}
}
