// Package redis provides a minimal RESP2 client using only the stdlib net package.
// Only the commands needed by the cache adapter are implemented: GET, SET EX, PING.
package redis

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrNil is returned by Get when the key does not exist in Redis.
var ErrNil = errors.New("redis: nil")

// Client is a single-connection Redis client with reconnect on failure.
// All methods are safe for concurrent use.
type Client struct {
	addr    string
	timeout time.Duration
	mu      sync.Mutex
	conn    net.Conn
	rdr     *bufio.Reader
}

// New returns a Client that will connect (lazily) to addr.
// addr should be "host:port", e.g. "localhost:6379".
func New(addr string) *Client {
	return &Client{
		addr:    addr,
		timeout: 5 * time.Second,
	}
}

// Ping sends PING and returns nil on PONG.
func (c *Client) Ping() error {
	_, err := c.do("PING")
	return err
}

// Get fetches the value for key. Returns ErrNil when the key does not exist.
func (c *Client) Get(key string) ([]byte, error) {
	v, err := c.do("GET", key)
	return v, err
}

// Set stores value at key with the given TTL.
func (c *Client) Set(key string, value []byte, ttl time.Duration) error {
	secs := int(ttl.Seconds())
	if secs < 1 {
		secs = 1
	}
	_, err := c.do("SET", key, string(value), "EX", strconv.Itoa(secs))
	return err
}

// do sends a RESP2 command and returns the bulk-string response.
func (c *Client) do(args ...string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ensureConn(); err != nil {
		return nil, err
	}

	if err := c.writeCmd(args...); err != nil {
		c.closeConn()
		return nil, fmt.Errorf("redis write: %w", err)
	}

	v, err := c.readReply()
	if err != nil {
		c.closeConn()
		return nil, err
	}
	return v, nil
}

// ── connection management ─────────────────────────────────────────────────────

func (c *Client) ensureConn() error {
	if c.conn != nil {
		return nil
	}
	conn, err := net.DialTimeout("tcp", c.addr, c.timeout)
	if err != nil {
		return fmt.Errorf("redis dial %s: %w", c.addr, err)
	}
	c.conn = conn
	c.rdr = bufio.NewReader(conn)
	return nil
}

func (c *Client) closeConn() {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
		c.rdr = nil
	}
}

// ── RESP2 encoding / decoding ─────────────────────────────────────────────────

// writeCmd serialises args as a RESP2 inline array and writes it to the connection.
func (c *Client) writeCmd(args ...string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(a), a)
	}
	deadline := time.Now().Add(c.timeout)
	c.conn.SetWriteDeadline(deadline)
	_, err := fmt.Fprint(c.conn, b.String())
	return err
}

// readReply reads one RESP2 reply from the connection.
func (c *Client) readReply() ([]byte, error) {
	c.conn.SetReadDeadline(time.Now().Add(c.timeout))
	line, err := c.rdr.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("redis read: %w", err)
	}
	line = strings.TrimRight(line, "\r\n")

	switch line[0] {
	case '+': // simple string
		return []byte(line[1:]), nil
	case '-': // error
		return nil, fmt.Errorf("redis error: %s", line[1:])
	case ':': // integer
		return []byte(line[1:]), nil
	case '$': // bulk string
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			return nil, fmt.Errorf("redis: malformed bulk length %q", line)
		}
		if n == -1 {
			return nil, ErrNil
		}
		buf := make([]byte, n+2) // +2 for \r\n
		if _, err := c.rdr.Read(buf); err != nil {
			return nil, fmt.Errorf("redis read bulk: %w", err)
		}
		return buf[:n], nil
	default:
		return nil, fmt.Errorf("redis: unknown reply type %q", line)
	}
}
