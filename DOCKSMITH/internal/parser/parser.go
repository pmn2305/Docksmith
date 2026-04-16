package parser

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type InstructionType string

const (
	FROM    InstructionType = "FROM"
	COPY    InstructionType = "COPY"
	RUN     InstructionType = "RUN"
	WORKDIR InstructionType = "WORKDIR"
	ENV     InstructionType = "ENV"
	CMD     InstructionType = "CMD"
)

type Instruction struct {
	Type    InstructionType
	Args    string
	LineNum int
}

func ParseFile(path string) ([]Instruction, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open Docksmithfile: %w", err)
	}
	defer f.Close()

	var instructions []Instruction
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		keyword := strings.ToUpper(parts[0])
		args := ""
		if len(parts) == 2 {
			args = strings.TrimSpace(parts[1])
		}
		switch InstructionType(keyword) {
		case FROM, COPY, RUN, WORKDIR, ENV, CMD:
			instructions = append(instructions, Instruction{
				Type:    InstructionType(keyword),
				Args:    args,
				LineNum: lineNum,
			})
		default:
			return nil, fmt.Errorf("line %d: unrecognised instruction %q", lineNum, keyword)
		}
	}
	return instructions, scanner.Err()
}

// ParseCMD parses ["exec","arg"] JSON array form
func ParseCMD(args string) ([]string, error) {
	var cmd []string
	if err := json.Unmarshal([]byte(args), &cmd); err != nil {
		return nil, fmt.Errorf("CMD must be a JSON array, e.g. [\"/bin/sh\",\"-c\",\"echo hi\"]: %w", err)
	}
	return cmd, nil
}

// ParseCOPY returns src, dest
func ParseCOPY(args string) (string, string, error) {
	parts := strings.Fields(args)
	if len(parts) < 2 {
		return "", "", fmt.Errorf("COPY requires <src> <dest>")
	}
	return parts[0], parts[len(parts)-1], nil
}

// ParseENV returns key, value
func ParseENV(args string) (string, string, error) {
	parts := strings.SplitN(args, "=", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("ENV must be KEY=VALUE")
	}
	return parts[0], parts[1], nil
}
