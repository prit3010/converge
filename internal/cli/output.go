package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/prit3010/converge/internal/config"
)

func writeJSONOutput(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode json output: %w", err)
	}
	return nil
}

type commandEnvelope struct {
	OK      bool          `json:"ok"`
	Command string        `json:"command"`
	Data    any           `json:"data,omitempty"`
	Error   *commandError `json:"error,omitempty"`
	Meta    envelopeMeta  `json:"meta"`
}

type envelopeMeta struct {
	SchemaVersion string `json:"schema_version"`
	Timestamp     string `json:"timestamp"`
}

func writeCommandSuccessJSON(w io.Writer, command string, data any) error {
	return writeJSONOutput(w, commandEnvelope{
		OK:      true,
		Command: command,
		Data:    data,
		Meta:    defaultEnvelopeMeta(),
	})
}

func writeCommandErrorJSON(w io.Writer, command string, err *commandError) error {
	if err == nil {
		err = internalErrorf("unknown error")
	}
	return writeJSONOutput(w, commandEnvelope{
		OK:      false,
		Command: command,
		Error:   err,
		Meta:    defaultEnvelopeMeta(),
	})
}

func defaultEnvelopeMeta() envelopeMeta {
	return envelopeMeta{
		SchemaVersion: config.DefaultJSONVersion,
		Timestamp:     time.Now().UTC().Format(time.RFC3339Nano),
	}
}
