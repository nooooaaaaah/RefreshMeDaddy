package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

// serverConfig holds the configuration for the server.
type serverConfig struct {
	port       string                                 // Port on which the server listens
	watchDir   string                                 // Directory to watch for changes
	verbose    bool                                   // Enable verbose logging
	ignoreList stringSlice                            // List of paths to ignore
	upgrader   websocket.Upgrader                     // Upgrader for websocket connections
	clients    map[*websocket.Conn]context.CancelFunc // Active clients and their cancel funcs
}

// stringSlice is a custom type that implements flag.Value interface for string slices.
type stringSlice []string

// String returns the string representation of the stringSlice.
func (i *stringSlice) String() string {
	return fmt.Sprint(*i)
}

// Set splits a comma-separated string and appends it to the slice.
func (i *stringSlice) Set(value string) error {
	for _, val := range strings.Split(value, ",") {
		*i = append(*i, val)
	}
	return nil
}

// init attempts to load environment variables from a .env file.
func init() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}
}

// main sets up the server configuration, starts the file watcher and the web server.
func main() {
	// Configuration and flag parsing
	var cfg serverConfig
	// Server configuration flags
	flag.StringVar(&cfg.port, "port", "8080", "port to run the WebSocket server on")
	flag.StringVar(&cfg.port, "p", "8080", "port to run the WebSocket server on (shorthand)")
	flag.StringVar(&cfg.watchDir, "watch", ".", "directory to watch for changes")
	flag.StringVar(&cfg.watchDir, "w", ".", "directory to watch for changes (shorthand)")
	flag.BoolVar(&cfg.verbose, "verbose", false, "enable verbose logging")
	flag.BoolVar(&cfg.verbose, "v", false, "enable verbose logging (shorthand)")
	flag.Var(&cfg.ignoreList, "ignore", "comma-separated list of directories or files to ignore")
	flag.Var(&cfg.ignoreList, "i", "comma-separated list of directories or files to ignore (shorthand)")
	flag.Parse()

	// Initialize clients map and upgrader configuration
	cfg.clients = make(map[*websocket.Conn]context.CancelFunc)
	cfg.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		// CheckOrigin verifies the origin of the request
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	// Setup signal handling for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// WebSocket handler
	http.HandleFunc("/refreshMeDaddy", func(w http.ResponseWriter, r *http.Request) {
		serveWs(&cfg, w, r)
	})
	// Start watching files in a separate goroutine
	go watchFiles(&cfg, ctx)

	// Server startup logs
	if cfg.verbose {
		log.Printf("Verbose logging enabled\n")
	}
	log.Printf("Starting live-reload server on :%s\n", cfg.port)
	server := &http.Server{Addr: ":" + cfg.port}
	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe(): %v", err)
		}
	}()

	<-ctx.Done() // Wait for interrupt signal to gracefully shutdown

	log.Println("Shutting down server...")
	if err := server.Shutdown(context.Background()); err != nil {
		log.Fatalf("Server Shutdown Failed:%+v", err)
	}
	log.Println("Server gracefully stopped")
}

// serveWs handles incoming WebSocket connections.
func serveWs(cfg *serverConfig, w http.ResponseWriter, r *http.Request) {
	// Upgrade HTTP server connection to a WebSocket connection
	conn, err := cfg.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	if cfg.verbose {
		log.Println("WebSocket connection established")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cfg.clients[conn] = cancel

	// Listen for messages on the WebSocket connection
	go func() {
		defer func() {
			conn.Close()
			delete(cfg.clients, conn)
			cancel()
			if cfg.verbose {
				log.Println("WebSocket connection closed")
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			default:
				if _, _, err := conn.NextReader(); err != nil {
					if cfg.verbose {
						log.Printf("WebSocket read error: %v", err)
					}
					return
				}
			}
		}
	}()
}

// watchFiles watches for file changes in the specified directory and notifies connected clients.
func watchFiles(cfg *serverConfig, ctx context.Context) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Close()

	// addDir recursively adds directories to the watcher, ignoring specified paths
	var addDir func(dir string) error
	addDir = func(dir string) error {
		if shouldIgnore(cfg, dir) {
			if cfg.verbose {
				log.Printf("Ignoring directory: %s\n", dir)
			}
			return nil
		}
		contents, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, d := range contents {
			if d.IsDir() {
				path := filepath.Join(dir, d.Name())
				if err := watcher.Add(path); err != nil {
					return err
				}
				if cfg.verbose {
					log.Printf("Watching directory: %s\n", path)
				}
				if err := addDir(path); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if err := addDir(cfg.watchDir); err != nil {
		log.Fatalf("Failed to add directory to watcher: %v", err)
	}

	// Listen for file change events and errors
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if cfg.verbose {
				log.Println("Detected change:", event)
			}
			// Notify all connected clients to reload
			for client, cancel := range cfg.clients {
				err := client.WriteMessage(websocket.TextMessage, []byte("reload"))
				if err != nil {
					log.Printf("Error sending reload message: %v", err)
					cancel() // Cancel context on error
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Watcher error: %v", err)
		}
	}
}

// shouldIgnore checks if a path should be ignored based on the server configuration.
func shouldIgnore(cfg *serverConfig, path string) bool {
	for _, ignore := range cfg.ignoreList {
		if ignore == path {
			return true
		}
	}
	return false
}
