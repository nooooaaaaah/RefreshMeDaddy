package main

import (
	"context"
	"flag"
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

var (
	port     string
	watch    string
	verbose  bool
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			if err := godotenv.Load(); err != nil {
				log.Println("No .env file found")
			}
			allowedOrigins := strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",")
			if len(allowedOrigins) == 1 && allowedOrigins[0] == "" {
				log.Fatal("Environment variable ALLOWED_ORIGINS is not set or empty")
			}
			origin := r.Header.Get("Origin")
			for _, allowedOrigin := range allowedOrigins {
				if origin == allowedOrigin {
					return true
				}
			}
			return false
		},
	}
	clients = make(map[*websocket.Conn]context.CancelFunc) // connected clients with cancel funcs
)

func init() {
	flag.StringVar(&port, "port", "8080", "port to run the WebSocket server on (shorthand: -p)")
	flag.StringVar(&port, "p", "8080", "port to run the WebSocket server on (shorthand)")
	flag.StringVar(&watch, "watch", ".", "directory to watch for changes (shorthand: -w)")
	flag.StringVar(&watch, "w", ".", "directory to watch for changes (shorthand)")
	flag.BoolVar(&verbose, "verbose", false, "enable verbose logging (shorthand: -v)")
	flag.BoolVar(&verbose, "v", false, "enable verbose logging (shorthand)")
	flag.Parse()
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	http.HandleFunc("/ws", serveWs)
	go watchFiles(ctx, watch)

	if verbose {
		log.Printf("Verbose logging enabled\n")
	}
	log.Printf("Starting live-reload server on :%s\n", port)
	server := &http.Server{Addr: ":" + port}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe(): %v", err)
		}
	}()

	<-ctx.Done() // Wait for interrupt signal

	stop()
	log.Println("Shutting down server...")
	if err := server.Shutdown(context.Background()); err != nil {
		log.Fatalf("Server Shutdown Failed:%+v", err)
	}
	log.Println("Server gracefully stopped")
}

func serveWs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	if verbose {
		log.Println("WebSocket connection established")
	}
	ctx, cancel := context.WithCancel(context.Background())
	clients[conn] = cancel

	go func() {
		defer func() {
			conn.Close()
			delete(clients, conn)
			cancel()
			if verbose {
				log.Println("WebSocket connection closed")
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			default:
				if _, _, err := conn.NextReader(); err != nil {
					if verbose {
						log.Println("WebSocket read error:", err)
					}
					return
				}
			}
		}
	}()
}

func watchFiles(ctx context.Context, directory string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal("Failed to create watcher:", err)
	}
	defer watcher.Close()

	// Function to recursively add directories to the watcher
	var addDir func(dir string) error
	addDir = func(dir string) error {
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
				if verbose {
					log.Printf("Watching directory: %s\n", path)
				}
				if err := addDir(path); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if err := addDir(directory); err != nil {
		log.Fatal("Failed to add directory to watcher:", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if verbose {
				log.Println("Detected change:", event)
			}
			for client, cancel := range clients {
				err := client.WriteMessage(websocket.TextMessage, []byte("reload"))
				if err != nil {
					log.Println("Error sending reload message:", err)
					cancel() // Cancel context on error
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Println("Watcher error:", err)
		}
	}
}
