package gostream

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// A Command describes an action for the server side of
// a connection to take.
type Command struct {
	Name string
	Args []string
	Raw  string
}

// ErrCommandMalformed happens if deserialization of a command fails.
var ErrCommandMalformed = errors.New("malformed command")

// UnmarshalCommand attempts to parse a space delimited command
// into its constituent parts; returns error if it is malformed.
func UnmarshalCommand(data string) (*Command, error) {
	parts := strings.Split(data, " ")
	if len(parts) == 0 {
		return nil, ErrCommandMalformed
	}
	return &Command{
		Name: parts[0],
		Args: parts[1:],
		Raw:  data,
	}, nil
}

// A CommandProcessor is able to process a command and deliver a response.
type CommandProcessor func(cmd *Command) (*CommandResponse, error)

// A CommandRegistry is responsible for processing registered commands.
type CommandRegistry interface {
	// Add associates a CommandProcessor with a command name
	Add(name string, processor CommandProcessor)
	// Process processes a Command associated with a name and processor in the registry.
	Process(cmd *Command) (*CommandResponse, error)
}

// NewCommandRegistry returns a simple, thread-safe CommandRegistry.
func NewCommandRegistry() CommandRegistry {
	return &commandRegistry{
		reg: map[string]CommandProcessor{},
	}
}

type commandRegistry struct {
	mu  sync.Mutex
	reg map[string]CommandProcessor
}

func (cr *commandRegistry) Add(name string, processor CommandProcessor) {
	cr.mu.Lock()
	cr.reg[name] = processor
	cr.mu.Unlock()
}

func (cr *commandRegistry) Process(cmd *Command) (*CommandResponse, error) {
	cr.mu.Lock()
	processor, ok := cr.reg[cmd.Name]
	cr.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("unknown command %q", cmd.Name)
	}
	return processor(cmd)
}

// A CommandResponse is the result of processing a command. Currently
// only the server side understands the structure of a response. In the future
// this should likely be a JSON based response that indicates success/failure along
// with accompanying data in the event of success or an error.
type CommandResponse struct {
	data   []byte
	isText bool
}

// NewCommandResponseText creates a CommandResponse with textual data.
func NewCommandResponseText(text string) *CommandResponse {
	return &CommandResponse{[]byte(text), true}
}
