package control

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/niski84/pulse-eight-cec-linux/internal/cec"
)

type Handler func(context.Context, cec.Action) error

type Server struct {
	path    string
	handler Handler
	ln      net.Listener
}

func New(path string, handler Handler) (*Server, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("control socket path is empty")
	}
	if handler == nil {
		return nil, errors.New("control handler is nil")
	}
	return &Server{path: path, handler: handler}, nil
}

func (s *Server) Run(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return fmt.Errorf("create control socket directory: %w", err)
	}
	_ = os.Remove(s.path)
	listener, err := net.Listen("unix", s.path)
	if err != nil {
		return fmt.Errorf("listen on control socket: %w", err)
	}
	s.ln = listener
	if err := os.Chmod(s.path, 0600); err != nil {
		_ = listener.Close()
		return fmt.Errorf("protect control socket: %w", err)
	}
	go func() {
		<-ctx.Done()
		_ = listener.Close()
		_ = os.Remove(s.path)
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go s.handle(ctx, conn)
	}
}

func (s *Server) handle(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	var request struct {
		Action string `json:"action"`
	}
	response := map[string]any{"ok": false}
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&request); err != nil {
		response["error"] = err.Error()
	} else if err := s.handler(ctx, cec.Action(strings.ToLower(strings.TrimSpace(request.Action)))); err != nil {
		response["error"] = err.Error()
	} else {
		response["ok"] = true
	}
	_ = json.NewEncoder(conn).Encode(response)
}

func Send(path string, action cec.Action) error {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(map[string]string{"action": string(action)}); err != nil {
		return err
	}
	var response struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&response); err != nil {
		return err
	}
	if !response.OK {
		return errors.New(response.Error)
	}
	return nil
}
