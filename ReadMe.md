# Live Reload Server for Development

This Go application serves as a live-reload server, enabling developers to automatically refresh web pages in the browser when changes are detected in the watched directory. It's particularly useful during the development of web applications, as it improves efficiency by eliminating the need to manually refresh the browser after each change.

## Features

- **WebSocket Communication:** Utilizes WebSockets to communicate reload commands to the client.
- **File Watching:** Watches for changes in a specified directory using `fsnotify`.
- **Environment Variable Support:** Reads allowed origins for WebSocket connections from environment variables, enhancing security.
- **Verbose Logging:** Offers an option for verbose logging to aid in debugging.

## Setup

### Prerequisites

- Go 1.16 or higher
- Access to modify environment variables

### Installation

1. **Clone or download this repository** to your local machine.
2. **Navigate to the directory** containing the source code.

### Configuration

1. **Environment Variables:** Create a `.env` file in the same directory as the executable or set environment variables in your system. Required variables:

   - `ALLOWED_ORIGINS`: Comma-separated list of allowed origins for WebSocket connections (e.g., `http://localhost:8080,http://localhost:3000`).

2. **Build the application:**

   ```bash
   go build -o live-reload-server
   ```

### Running the Server

Execute the compiled binary with optional flags:

```bash
./live-reload-server -p 8080 -w ./path/to/watch -v
```

- `-p` or `--port`: Port to run the WebSocket server on.
- `-w` or `--watch`: Directory to watch for changes.
- `-v` or `--verbose`: Enable verbose logging.

### Integrating with the Client

Ensure your client-side application is configured to establish a WebSocket connection to the server:

```javascript
const socket = new WebSocket("ws://localhost:8080/ws");
socket.onmessage = (message) => {
  if (message.data === "reload") {
    window.location.reload();
  }
};
```

## Usage

Once the server is running and your client-side application is configured to listen for reload messages, any change within the watched directory triggers an automatic page reload in the browser.

## Note

- Ensure that the `ALLOWED_ORIGINS` environment variable accurately reflects the origins from which you'll be serving your client-side application to avoid WebSocket connection issues.
- Currently no support for ignoring file types. Only Files and Directories at the moment.

## Contribution

Contributions are welcome! Please submit a pull request or open an issue if you have any improvements or encounter any problems.

---

This server is designed for development use and should not be used in production environments. Always ensure that your development tools are securely configured.
