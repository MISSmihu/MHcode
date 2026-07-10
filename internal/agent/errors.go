package agent

import "fmt"

type UnknownReasoningLevelError struct {
	Level ReasoningLevel
}

func (e *UnknownReasoningLevelError) Error() string {
	return fmt.Sprintf("unknown reasoning level %q", e.Level)
}
