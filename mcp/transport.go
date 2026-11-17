package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

type envelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

func idKey(id json.RawMessage) string {
	if !parseID(id) {
		return ""
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(id))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return "string:" + v
	case json.Number:
		raw := string(v)
		if len(raw) > 128 {
			return "number:" + raw
		}
		if i := strings.IndexAny(raw, "eE"); i >= 0 {
			exp, e := strconv.Atoi(raw[i+1:])
			if e != nil || exp > 1000 || exp < -1000 {
				return "number:" + raw
			}
		}
		if r, ok := new(big.Rat).SetString(raw); ok {
			return "number:" + r.RatString()
		}
		return ""
	default:
		return "null"
	}
}
func readLines(in io.Reader, max int, onLine func([]byte) error, onLarge func() error) error {
	reader := bufio.NewReaderSize(in, 64<<10)
	line := []byte{}
	discarding := false
	for {
		part, e := reader.ReadSlice('\n')
		if !discarding {
			if len(line)+len(part) > max+1 {
				line = nil
				discarding = true
			} else {
				line = append(line, part...)
			}
		}
		complete := e == nil || e == io.EOF
		if complete {
			if discarding {
				if err := onLarge(); err != nil {
					return err
				}
			} else if len(bytes.TrimSpace(line)) > 0 {
				trimmed := bytes.TrimSuffix(line, []byte("\n"))
				trimmed = bytes.TrimSuffix(trimmed, []byte("\r"))
				if len(trimmed) > max {
					if err := onLarge(); err != nil {
						return err
					}
				} else if err := onLine(trimmed); err != nil {
					return err
				}
			}
			line = nil
			discarding = false
		}
		if e == io.EOF {
			return nil
		}
		if e != nil && e != bufio.ErrBufferFull {
			return e
		}
	}
}

type activeRequest struct {
	cancel context.CancelFunc
	token  uint64
}
type job struct {
	line   []byte
	ctx    context.Context
	key    string
	token  uint64
	cancel context.CancelFunc
}

// Serve reads NDJSON, dispatches bounded concurrent requests and drains accepted work at EOF.
// Cancellation closes streams implementing io.Closer to unblock pending I/O.
func (s *Server) Serve(parent context.Context, in io.Reader, out, log io.Writer) error {
	if in == nil || out == nil {
		return fmt.Errorf("MCP input and output are required")
	}
	if log == nil {
		log = io.Discard
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	stop := context.AfterFunc(ctx, func() {
		if closer, ok := in.(io.Closer); ok {
			_ = closer.Close()
		}
		if closer, ok := out.(io.Closer); ok {
			_ = closer.Close()
		}
	})
	defer stop()

	var writeMu sync.Mutex
	var writeErr error
	write := func(response *Response) error {
		if response == nil {
			return nil
		}
		body, e := encode(response)
		if e != nil {
			return e
		}
		body = append(body, '\n')
		writeMu.Lock()
		defer writeMu.Unlock()
		if writeErr != nil {
			return writeErr
		}
		n, e := out.Write(body)
		if e == nil && n != len(body) {
			e = io.ErrShortWrite
		}
		if e != nil {
			writeErr = e
			cancel()
		}
		return e
	}
	fmt.Fprintf(log, "[stock-go mcp] ready · %d tools · %d prompts · stock-go@%s\n", len(s.tools), len(s.prompts), s.version)
	for _, warning := range s.warnings {
		fmt.Fprintln(log, "[stock-go mcp]", warning)
	}
	active := map[string]activeRequest{}
	var activeMu sync.Mutex
	var next uint64
	jobs := make(chan job, s.pending)
	var workers sync.WaitGroup
	finish := func(j job) {
		j.cancel()
		if j.key != "" {
			activeMu.Lock()
			if current, ok := active[j.key]; ok && current.token == j.token {
				delete(active, j.key)
			}
			activeMu.Unlock()
		}
	}
	for i := 0; i < s.workers; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := range jobs {
				response := s.Dispatch(j.ctx, j.line)
				_ = write(response)
				finish(j)
			}
		}()
	}
	onLine := func(line []byte) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		var msg envelope
		decodeErr := json.Unmarshal(line, &msg)
		validNotification := decodeErr == nil && msg.JSONRPC == "2.0" && strings.TrimSpace(msg.Method) != "" && len(msg.ID) == 0
		if validNotification && msg.Method == "notifications/cancelled" {
			var params struct {
				RequestID json.RawMessage `json:"requestId"`
			}
			if json.Unmarshal(msg.Params, &params) == nil {
				key := idKey(params.RequestID)
				activeMu.Lock()
				if request, ok := active[key]; ok {
					request.cancel()
				}
				activeMu.Unlock()
			}
			return nil
		}
		if validNotification {
			return nil
		}
		requestCtx, requestCancel := context.WithCancel(ctx)
		key := idKey(msg.ID)
		j := job{line: append([]byte{}, line...), ctx: requestCtx, key: key, cancel: requestCancel}
		if key != "" {
			activeMu.Lock()
			if _, exists := active[key]; exists {
				activeMu.Unlock()
				requestCancel()
				return write(rpcError(msg.ID, InvalidRequest, "Duplicate in-flight request id"))
			}
			next++
			j.token = next
			active[key] = activeRequest{cancel: requestCancel, token: j.token}
			activeMu.Unlock()
		}
		select {
		case jobs <- j:
			return nil
		case <-ctx.Done():
			finish(j)
			return ctx.Err()
		default:
			finish(j)
			if len(msg.ID) > 0 && parseID(msg.ID) {
				return write(rpcError(msg.ID, InternalError, "Server busy; retry later"))
			}
			return write(rpcError(nil, InternalError, "Server busy; retry later"))
		}
	}
	e := readLines(in, s.maxLine, onLine, func() error {
		return write(rpcError(nil, InvalidRequest, fmt.Sprintf("Message too large (limit %d bytes)", s.maxLine)))
	})
	if e != nil {
		cancel()
	}
	close(jobs)
	workers.Wait()
	if writeErr != nil {
		if errors.Is(writeErr, syscall.EPIPE) {
			return nil
		}
		if parent.Err() != nil {
			return parent.Err()
		}
		return writeErr
	}
	if parent.Err() != nil {
		return parent.Err()
	}
	if ctx.Err() != nil && e != nil && strings.Contains(e.Error(), "closed") {
		return ctx.Err()
	}
	return e
}

// Start constructs a server from explicit options/environment and serves the supplied streams.
func Start(ctx context.Context, in io.Reader, out, log io.Writer, options ...Options) error {
	s, e := New(options...)
	if e != nil {
		return e
	}
	return s.Serve(ctx, in, out, log)
}
