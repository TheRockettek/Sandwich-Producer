package gateway

import (
	"sync"

	"github.com/gorilla/websocket"
)

// Connection wraps a websocket
type Connection struct {
	ws   *websocket.Conn
	rmux *sync.Mutex
	wmux *sync.Mutex
}

// NewConnection creates a wrapper arround a websocket connection
func NewConnection(conn *websocket.Conn) (c *Connection) {
	return &Connection{
		ws:   conn,
		rmux: &sync.Mutex{},
		wmux: &sync.Mutex{},
	}
}

// CloseWithCode closes the connection with a specified code
func (c *Connection) CloseWithCode(code int) error {
	return c.ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(code, "Normal Closure"))
}

// Close closes the connection
func (c *Connection) Close() error {
	return c.CloseWithCode(websocket.CloseNormalClosure)
}

func (c *Connection) Write(d []byte) (int, error) {
	c.wmux.Lock()
	defer c.wmux.Unlock()

	return len(d), c.ws.WriteMessage(websocket.BinaryMessage, d)
}

func (c *Connection) Read() (d []byte, err error) {
	c.rmux.Lock()
	defer c.rmux.Unlock()

	t, d, err := c.ws.ReadMessage()
	if err != nil {
		return
	}

	if t == websocket.BinaryMessage {
		// d, err = c.compressor.Decompress(d)
	}

	return
}
