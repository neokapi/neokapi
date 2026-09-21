package host

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// parentProcess reads one process's parent out of /proc.
//
// `/proc/<pid>/stat` holds the command name in parentheses, and the name may
// itself contain spaces and parentheses, so the fields after it are counted
// from the last closing parenthesis rather than by splitting the whole line.
// Field 2 of what follows is the parent pid and field 20 is the start time.
func parentProcess(pid int) (processInfo, error) {
	ppid, _, _, err := readProcStat(pid)
	if err != nil {
		return processInfo{}, err
	}
	if ppid <= 0 {
		return processInfo{}, errNoAgentHostProcess
	}
	_, name, start, err := readProcStat(ppid)
	if err != nil {
		return processInfo{}, err
	}
	return processInfo{PID: ppid, Name: name, Start: start}, nil
}

// readProcStat reports one process's parent pid, command name and start time.
func readProcStat(pid int) (ppid int, name string, start int64, err error) {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, "", 0, err
	}
	line := string(raw)
	opened := strings.IndexByte(line, '(')
	closed := strings.LastIndexByte(line, ')')
	if opened < 0 || closed < opened {
		return 0, "", 0, fmt.Errorf("host: /proc/%d/stat holds no command name", pid)
	}
	name = line[opened+1 : closed]
	fields := strings.Fields(line[closed+1:])
	// state, ppid, pgrp, … : the parent is the second field after the name,
	// and the start time the twentieth.
	if len(fields) < 20 {
		return 0, "", 0, fmt.Errorf("host: /proc/%d/stat holds %d fields", pid, len(fields))
	}
	ppid, err = strconv.Atoi(fields[1])
	if err != nil {
		return 0, "", 0, fmt.Errorf("host: /proc/%d/stat parent: %w", pid, err)
	}
	start, err = strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return 0, "", 0, fmt.Errorf("host: /proc/%d/stat start time: %w", pid, err)
	}
	return ppid, name, start, nil
}
