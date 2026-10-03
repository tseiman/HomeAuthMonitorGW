package nut

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/tseiman/HomeAuthMonitorGW/internal/metrics"
)

type Client struct {
	Address string
	Timeout time.Duration
}

func (c Client) ListVariables(ctx context.Context, ups string) (map[string]string, error) {
	var d net.Dialer
	d.Timeout = c.Timeout
	conn, err := d.DialContext(ctx, "tcp", c.Address)
	if err != nil {
		return nil, fmt.Errorf("connect to upsd: %w", err)
	}
	defer conn.Close()
	deadline := time.Now().Add(c.Timeout)
	_ = conn.SetDeadline(deadline)
	if _, err := io.WriteString(conn, "LIST VAR "+ups+"\n"); err != nil {
		return nil, fmt.Errorf("query upsd: %w", err)
	}
	return ParseListVAR(bufio.NewReader(io.LimitReader(conn, 1<<20)), ups)
}

func ParseListVAR(r *bufio.Reader, ups string) (map[string]string, error) {
	values := make(map[string]string)
	started := false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, errors.New("incomplete NUT response")
			}
			return nil, fmt.Errorf("read NUT response: %w", err)
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ERR ") {
			return nil, fmt.Errorf("upsd returned %s", strings.TrimPrefix(line, "ERR "))
		}
		if !started {
			if line != "BEGIN LIST VAR "+ups {
				return nil, errors.New("unexpected NUT response")
			}
			started = true
			continue
		}
		if line == "END LIST VAR "+ups {
			return values, nil
		}
		parts := strings.SplitN(line, " ", 4)
		if len(parts) != 4 || parts[0] != "VAR" || parts[1] != ups {
			return nil, errors.New("malformed NUT variable")
		}
		value, err := strconv.Unquote(parts[3])
		if err != nil {
			return nil, errors.New("malformed NUT value")
		}
		values[parts[2]] = value
	}
}

type VariableClient interface {
	ListVariables(context.Context, string) (map[string]string, error)
}
type Collector struct {
	Client VariableClient
	UPS    string
	Now    func() time.Time
}

func (c Collector) Poll(ctx context.Context) ([]metrics.Metric, error) {
	vars, err := c.Client.ListVariables(ctx, c.UPS)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if c.Now != nil {
		now = c.Now().UTC()
	}
	out := make([]metrics.Metric, 0, len(vars))
	for name, raw := range vars {
		value, typ := parseValue(raw)
		out = append(out, metrics.Metric{Name: name, Value: value, ValueType: typ, Timestamp: now})
	}
	sortMetrics(out)
	return out, nil
}
func parseValue(raw string) (any, string) {
	if v, err := strconv.ParseFloat(raw, 64); err == nil {
		return v, "number"
	}
	return raw, "string"
}
func sortMetrics(v []metrics.Metric) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j].Name < v[j-1].Name; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}
