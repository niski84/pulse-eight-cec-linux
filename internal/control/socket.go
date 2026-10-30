package control

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/niski84/pulse-eight-cec-linux/internal/cec"
	"github.com/niski84/pulse-eight-cec-linux/internal/policy"
)

type Handler func(context.Context, cec.Action) error

// Status describes the running bridge, not just installed prerequisites.
type Status struct {
	AdapterReady  bool   `json:"adapter_ready"`
	InputReady    bool   `json:"input_ready"`
	Shadow        bool   `json:"shadow"`
	Received      uint64 `json:"received_keys"`
	Forwarded     uint64 `json:"forwarded_keys"`
	Unsupported   uint64 `json:"unsupported_keys"`
	Ignored       uint64 `json:"ignored_keys"`
	ActiveProfile string `json:"active_profile,omitempty"`
}

type response struct {
	OK       bool              `json:"ok"`
	Error    string            `json:"error,omitempty"`
	Status   *Status           `json:"status,omitempty"`
	Profile  string            `json:"active_profile,omitempty"`
	Profiles []string          `json:"profiles,omitempty"`
	Trace    []policy.Decision `json:"trace,omitempty"`
}

type Server struct {
	path    string
	handler Handler
	ln      net.Listener
	status  func() Status
	policy  *policy.Engine
}

func (s *Server) WithPolicy(engine *policy.Engine) *Server {
	s.policy = engine
	return s
}

func (s *Server) WithStatus(status func() Status) *Server {
	s.status = status
	return s
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
	// Serialize stale-socket recovery as well as normal endpoint ownership.
	// Keep the lock file's inode after shutdown so contenders cannot lock a
	// replacement inode while another server is still using the old one.
	lock, err := os.OpenFile(s.path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("open control lock: %w", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("control endpoint already owned: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	listener, err := net.Listen("unix", s.path)
	if errors.Is(err, syscall.EADDRINUSE) {
		// Recover only a stale socket. Never unlink a live daemon's endpoint,
		// a regular file, or a socket that failed for a permissions reason.
		conn, dialErr := net.DialTimeout("unix", s.path, time.Second)
		if dialErr == nil {
			_ = conn.Close()
			return errors.New("control socket is already owned by a running daemon")
		}
		info, statErr := os.Lstat(s.path)
		if errors.Is(dialErr, syscall.ECONNREFUSED) && statErr == nil && info.Mode()&os.ModeSocket != 0 {
			if err := os.Remove(s.path); err != nil {
				return fmt.Errorf("remove stale control socket: %w", err)
			}
			listener, err = net.Listen("unix", s.path)
		}
	}
	if err != nil {
		return fmt.Errorf("listen on control socket: %w", err)
	}
	s.ln = listener
	defer listener.Close()
	if err := os.Chmod(s.path, 0600); err != nil {
		_ = listener.Close()
		return fmt.Errorf("protect control socket: %w", err)
	}
	go func() {
		<-ctx.Done()
		_ = listener.Close()
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
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		log.Printf("control socket deadline: %v", err)
		return
	}
	var request struct {
		Command string `json:"command"`
		Action  string `json:"action"`
		Profile string `json:"profile"`
		Limit   int    `json:"limit"`
	}
	reply := response{}
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&request); err != nil {
		reply.Error = err.Error()
	} else if request.Command == "status" && s.status != nil {
		status := s.status()
		reply.OK, reply.Status = true, &status
	} else if request.Command == "profiles" && s.policy != nil {
		reply.OK, reply.Profiles = true, s.policy.Profiles()
	} else if request.Command == "profile" && s.policy != nil {
		if request.Profile != "" {
			if err := s.policy.SetProfile(request.Profile); err != nil {
				reply.Error = err.Error()
			}
		}
		if reply.Error == "" {
			reply.OK, reply.Profile = true, s.policy.ActiveProfile()
		}
	} else if request.Command == "trace" && s.policy != nil {
		if request.Limit < 0 || request.Limit > 50 {
			reply.Error = "trace limit must be between 0 and 50"
		} else {
			reply.OK, reply.Trace = true, s.policy.Trace(request.Limit)
		}
	} else if request.Command != "" && request.Command != "key" {
		reply.Error = "unsupported command"
	} else if err := s.handler(ctx, cec.Action(strings.ToLower(strings.TrimSpace(request.Action)))); err != nil {
		reply.Error = err.Error()
	} else {
		reply.OK = true
	}
	if err := json.NewEncoder(conn).Encode(reply); err != nil {
		log.Printf("control socket reply: %v", err)
	}
}

func Send(path string, action cec.Action) error {
	_, err := exchange(path, map[string]string{"command": "key", "action": string(action)})
	return err
}

func ReadStatus(path string) (Status, error) {
	reply, err := exchange(path, map[string]string{"command": "status"})
	if err != nil {
		return Status{}, err
	}
	if reply.Status == nil {
		return Status{}, errors.New("daemon did not return live status")
	}
	return *reply.Status, nil
}

func ReadProfiles(path string) ([]string, error) {
	reply, err := exchange(path, map[string]string{"command": "profiles"})
	return reply.Profiles, err
}

// SelectProfile queries the active activity when name is empty, or changes it.
func SelectProfile(path, name string) (string, error) {
	reply, err := exchange(path, map[string]string{"command": "profile", "profile": name})
	return reply.Profile, err
}

func ReadTrace(path string, limit int) ([]policy.Decision, error) {
	reply, err := exchange(path, map[string]any{"command": "trace", "limit": limit})
	if err != nil {
		return nil, err
	}
	if reply.Trace == nil {
		return []policy.Decision{}, nil
	}
	return reply.Trace, nil
}

func exchange(path string, request any) (response, error) {
	conn, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		return response{}, err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return response{}, err
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return response{}, err
	}
	var reply response
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&reply); err != nil {
		return response{}, err
	}
	if !reply.OK {
		return response{}, errors.New(reply.Error)
	}
	return reply, nil
}
