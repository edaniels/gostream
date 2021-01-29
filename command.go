package gostream

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

type Command struct {
	Name string
	Args []string
	Raw  string
}

func UnmarshalCommand(data string) (*Command, error) {
	parts := strings.Split(data, " ")
	if len(parts) == 0 {
		return nil, errors.New("malformed command")
	}
	return &Command{
		Name: parts[0],
		Args: parts[1:],
		Raw:  data,
	}, nil
}

type CommandProcessor func(cmd *Command) (*CommandResponse, error)

type CommandRegistry interface {
	Add(name string, processor CommandProcessor)
	Process(cmd *Command) (*CommandResponse, error)
}

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

type CommandResponse struct {
	data   []byte
	isText bool
}

func NewCommandResponseText(text string) *CommandResponse {
	return &CommandResponse{[]byte(text), true}
}
