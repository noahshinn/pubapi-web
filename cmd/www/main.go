package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"search_engine/www"
	"sync"
	"syscall"
)

type Server struct {
	LocalPath string
	Port      int
	Cmd       *exec.Cmd
}

type ServerManager struct {
	Servers   []*Server
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	errChan   chan error
	endpoints []*www.Endpoint
}

func NewServerManager(ctx context.Context, directories []string, startingPort int) (*ServerManager, error) {
	ctx, cancel := context.WithCancel(ctx)
	manager := &ServerManager{
		Servers: make([]*Server, len(directories)),
		ctx:     ctx,
		cancel:  cancel,
		errChan: make(chan error, len(directories)),
	}
	endpoints := []*www.Endpoint{}
	for i, dir := range directories {
		port := startingPort + i
		server := &Server{
			LocalPath: dir,
			Port:      port,
		}
		manager.Servers[i] = server
		endpoints = append(endpoints, &www.Endpoint{Protocol: "http", IpAddress: "localhost", Port: port, Path: "/"})
	}
	manager.endpoints = endpoints
	return manager, nil
}

func (sm *ServerManager) Start() error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	for _, server := range sm.Servers {
		sm.wg.Add(1)
		go sm.runServer(server)
	}

	go func() {
		select {
		case <-sm.ctx.Done():
		case sig := <-sigChan:
			fmt.Printf("\nReceived signal: %v\n", sig)
			sm.cancel()
		}
		for _, server := range sm.Servers {
			if server.Cmd != nil && server.Cmd.Process != nil {
				fmt.Printf("Shutting down server in %s\n", server.LocalPath)
				err := server.Cmd.Process.Signal(syscall.SIGTERM)
				if err != nil {
					sm.errChan <- fmt.Errorf("failed to send SIGTERM to server in %s: %v", server.LocalPath, err)
				}
			}
		}
	}()

	go func() {
		sm.wg.Wait()
		close(sm.errChan)
	}()

	select {
	case err := <-sm.errChan:
		if err != nil {
			sm.cancel()
			return fmt.Errorf("server error: %v", err)
		}
	default:
	}
	return nil
}

func (sm *ServerManager) Stop() error {
	sm.cancel()
	sm.wg.Wait()
	for err := range sm.errChan {
		if err != nil {
			return fmt.Errorf("server error during shutdown: %v", err)
		}
	}
	return nil
}

func (sm *ServerManager) runServer(server *Server) {
	defer sm.wg.Done()
	absPath, err := filepath.Abs(server.LocalPath)
	if err != nil {
		sm.errChan <- fmt.Errorf("failed to get absolute path for %s: %v", server.LocalPath, err)
		return
	}
	mainFile := filepath.Join(absPath, "main.go")
	if _, err := os.Stat(mainFile); os.IsNotExist(err) {
		sm.errChan <- fmt.Errorf("main.go not found in %s: %v", absPath, err)
		return
	}
	cmd := exec.CommandContext(sm.ctx, "go", "run", mainFile, "--port", fmt.Sprintf("%d", server.Port))
	cmd.Dir = absPath
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr
	server.Cmd = cmd
	if err := cmd.Start(); err != nil {
		sm.errChan <- fmt.Errorf("failed to start server in %s: %v", server.LocalPath, err)
		return
	}
	if err := cmd.Wait(); err != nil {
		if sm.ctx.Err() == nil {
			sm.errChan <- fmt.Errorf("server in %s exited with error: %v", server.LocalPath, err)
		}
	}
}

func main() {
	servers := flag.String("servers", "", "path to a directory with servers to run locally")
	startingPort := flag.Int("starting-port", 8080, "port to start the servers from")
	maxNumServers := flag.Int("max-num-servers", -1, "maximum number of servers to run")
	flag.Parse()
	if *servers == "" {
		log.Fatal("servers path is required")
	}
	entries, err := os.ReadDir(*servers)
	if err != nil {
		log.Fatal(err)
	}
	var directories []string
	for _, entry := range entries {
		if entry.IsDir() {
			directories = append(directories, filepath.Join(*servers, entry.Name()))
		}
	}
	if *maxNumServers != -1 && len(directories) > *maxNumServers {
		directories = directories[:*maxNumServers]
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager, err := NewServerManager(ctx, directories, *startingPort)
	if err != nil {
		log.Fatal(err)
	}
	if err := manager.Start(); err != nil {
		log.Fatal(err)
	}
	bytes, err := json.Marshal(manager.endpoints)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("endpoints.json", bytes, 0644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Endpoints written to endpoints.json\n")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	if err := manager.Stop(); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}
}
