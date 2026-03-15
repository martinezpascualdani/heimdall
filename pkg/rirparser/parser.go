package rirparser

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// ParseResult holds the parsed header and a channel of records for streaming.
type ParseResult struct {
	Header  *Header
	Records <-chan *Record
	Err     <-chan error
}

// ParseStream reads an RIR delegated file from r. It parses the version line first,
// then streams records. Caller must consume Records and Err until closed.
// Streaming-friendly: does not load the whole file in memory.
func ParseStream(r io.Reader) (*ParseResult, error) {
	br := bufio.NewReader(r)
	// Find first non-comment, non-blank line (version line)
	var versionLine string
	for {
		line, err := br.ReadString('\n')
		if err == io.EOF && line == "" {
			return nil, fmt.Errorf("no version line found")
		}
		line = trimLine(line)
		if line == "" || strings.HasPrefix(line, "#") {
			if err == io.EOF {
				return nil, fmt.Errorf("no version line found")
			}
			continue
		}
		versionLine = line
		break
	}
	header, err := ParseHeader(versionLine)
	if err != nil {
		return nil, fmt.Errorf("parse header: %w", err)
	}
	records := make(chan *Record, 64)
	errCh := make(chan error, 1)
	go func() {
		defer close(records)
		defer close(errCh)
		for {
			line, err := br.ReadString('\n')
			if err != nil && err != io.EOF {
				errCh <- err
				return
			}
			rec, parseErr := ParseRecord(line)
			if parseErr != nil {
				errCh <- parseErr
				return
			}
			if rec != nil {
				records <- rec
			}
			if err == io.EOF {
				return
			}
		}
	}()
	return &ParseResult{
		Header:  header,
		Records: records,
		Err:     errCh,
	}, nil
}

func trimLine(s string) string {
	if len(s) > 0 && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	return trimSpace(s)
}

func trimSpace(s string) string {
	return strings.TrimSpace(s)
}
