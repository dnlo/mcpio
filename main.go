package main

import (
    "bufio"
    "flag"
    "fmt"
    "io"
    "os"
    "os/exec"
    "path/filepath"
    "regexp"
    "strings"
    "sync"
    "sync/atomic"
    "syscall"
)

type serverSpec struct {
    Name string
    Cmd  []string
}

// serverRuntime removed with daemon functionality

// Parse args after flags into groups separated by "--".
// Each group is: <name> <cmd> [args...]
func parseServers(args []string) ([]serverSpec, error) {
    var servers []serverSpec
    var group []string
    flush := func() error {
        if len(group) == 0 {
            return nil
        }
        if len(group) < 2 {
            return fmt.Errorf("server group %q requires at least a name and a command", strings.Join(group, " "))
        }
        spec := serverSpec{Name: group[0], Cmd: group[1:]}
        servers = append(servers, spec)
        group = nil
        return nil
    }
    for _, a := range args {
        if a == "--" {
            if err := flush(); err != nil {
                return nil, err
            }
            continue
        }
        group = append(group, a)
    }
    if err := flush(); err != nil {
        return nil, err
    }
    if len(servers) == 0 {
        return nil, fmt.Errorf("no servers specified")
    }
    return servers, nil
}

func sanitizeName(name string) string {
    // Allow alnum, dash, underscore; replace others with underscore
    re := regexp.MustCompile(`[^a-zA-Z0-9_-]+`)
    s := re.ReplaceAllString(name, "_")
    if s == "" {
        s = "server"
    }
    return s
}

// Start a single server process with named FIFO for stdin and log files for stdout/stderr.
func runServer(baseDir string, spec serverSpec, printAll bool, wg *sync.WaitGroup) {
    defer wg.Done()

    name := sanitizeName(spec.Name)
    // Single directory schema: baseDir/.mcpio with per-command files
    mcpDir := filepath.Join(baseDir, ".mcpio")
    if err := os.MkdirAll(mcpDir, 0o755); err != nil {
        fmt.Fprintf(os.Stderr, "%s: mkdir failed: %v\n", mcpDir, err)
        return
    }

    fifoPath := filepath.Join(mcpDir, name+".in.fifo")
    outLogPath := filepath.Join(mcpDir, name+".out.log")

    // Recreate FIFO
    if _, err := os.Lstat(fifoPath); err == nil {
        _ = os.Remove(fifoPath)
    }
    if err := syscall.Mkfifo(fifoPath, 0600); err != nil {
        fmt.Fprintf(os.Stderr, "%s: fifo create failed: %v\n", fifoPath, err)
        return
    }
    defer os.Remove(fifoPath)

    // Open combined out log (captures both stdout and stderr)
    outLog, err := os.OpenFile(outLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
    if err != nil {
        fmt.Fprintf(os.Stderr, "%s: open out log failed: %v\n", outLogPath, err)
        return
    }
    defer outLog.Close()

    // Start server and obtain pipes
    cmd := exec.Command(spec.Cmd[0], spec.Cmd[1:]...)
    stdin, err := cmd.StdinPipe()
    if err != nil {
        fmt.Fprintf(os.Stderr, "%s: stdin pipe failed: %v\n", name, err)
        return
    }
    stdout, err := cmd.StdoutPipe()
    if err != nil {
        fmt.Fprintf(os.Stderr, "%s: stdout pipe failed: %v\n", name, err)
        return
    }
    stderr, err := cmd.StderrPipe()
    if err != nil {
        fmt.Fprintf(os.Stderr, "%s: stderr pipe failed: %v\n", name, err)
        return
    }
    if err := cmd.Start(); err != nil {
        fmt.Fprintf(os.Stderr, "%s: start failed: %v\n", name, err)
        return
    }
    // Reap in background to avoid zombies
    go func() { _ = cmd.Wait() }()

    // stdin assigned locally

    // Emit helpful info in a distinct, non-IO format on a single line
    relIn, _ := filepath.Rel(baseDir, fifoPath)
    relOut, _ := filepath.Rel(baseDir, outLogPath)
    fmt.Fprintf(os.Stdout, "[%s:files]: %s %s\n", name, relIn, relOut)

    // Helper: line copier that writes to log and optionally to stdout with prefix
    copyWithPrefix := func(r io.Reader, logFile *os.File, kind string, alsoPrint bool) {
        scanner := bufio.NewScanner(r)
        for scanner.Scan() {
            line := scanner.Text()
            // write to log (append newline)
            fmt.Fprintln(logFile, line)
            if alsoPrint {
                fmt.Fprintf(os.Stdout, "[%s:%s] %s\n", name, kind, line)
            }
        }
        // If there is an error other than EOF, log it
        if err := scanner.Err(); err != nil && err != io.EOF {
            fmt.Fprintf(logFile, "[%s:%s] copier error: %v\n", name, kind, err)
        }
    }

    // Track whether any input has been sent yet (from FIFO)
    var gotInput atomic.Bool

    // Open FIFO read-write so open doesn't block; stream into child stdin
    fifo, err := os.OpenFile(fifoPath, os.O_RDWR, 0600)
    if err != nil {
        fmt.Fprintf(os.Stderr, "%s: fifo open failed: %v\n", name, err)
        return
    }
    defer fifo.Close()

    // Decide print modes
    // - Inputs and stdout mirror only when -print is set
    // - Stderr from the child is ALWAYS mirrored regardless of -print
    printIn := printAll
    printOut := printAll


    // Start piping FIFO to child stdin line-by-line to optionally mirror to stdout
    go func() {
        scanner := bufio.NewScanner(fifo)
        for scanner.Scan() {
            line := scanner.Text()
            io.WriteString(stdin, line+"\n")
            if !gotInput.Load() {
                gotInput.Store(true)
            }
            if printIn {
                fmt.Fprintf(os.Stdout, "[%s:in] %s\n", name, line)
            }
        }
        if err := scanner.Err(); err != nil && err != io.EOF {
            fmt.Fprintf(outLog, "[%s:in] copier error: %v\n", name, err)
        }
    }()

    // Copy stdout/stderr until EOF with optional printing
    var outWG sync.WaitGroup
    outWG.Add(2)
    go func() { defer outWG.Done(); copyWithPrefix(stdout, outLog, "out", printOut) }()
    // For stderr: always mirror until first input is sent; after that, mirror only when printAll
    go func() {
        defer outWG.Done()
        scanner := bufio.NewScanner(stderr)
        for scanner.Scan() {
            line := scanner.Text()
            fmt.Fprintln(outLog, line)
            if printAll || !gotInput.Load() {
                fmt.Fprintf(os.Stdout, "[%s:err] %s\n", name, line)
            }
        }
        if err := scanner.Err(); err != nil && err != io.EOF {
            fmt.Fprintf(outLog, "[%s:err] copier error: %v\n", name, err)
        }
    }()
    outWG.Wait()

    // When stdout/stderr complete, runServer returns
}

// Multi-server MCP bridge: spawns named servers, exposes per-server FIFOs, and logs stdout/stderr per server.
func main() {
    baseDir := flag.String("dir", ".", "base directory for .mcpio files")
    printAll := flag.Bool("print", false, "mirror in/out/err to stdout")
    flag.Parse()

    servers, err := parseServers(flag.Args())
    if err != nil {
        fmt.Fprintln(os.Stderr, "Usage: mcpio [flags] -- <name1> <cmd1> [args...] -- <name2> <cmd2> [args...] ...")
        fmt.Fprintln(os.Stderr, "Error:", err)
        os.Exit(2)
    }

    var wg sync.WaitGroup
    wg.Add(len(servers))
    for _, s := range servers {
        s := s
        go runServer(*baseDir, s, *printAll, &wg)
    }
    wg.Wait()
}
