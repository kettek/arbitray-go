package main

import (
  "fmt"
  "github.com/getlantern/systray"
  "github.com/gen2brain/dlgs"
  "sync"
  "io"
  "os"
  "syscall"
  "bufio"
  "log"
)

type Arbitray struct {
  config ArbitrayConfig
  waitGroup sync.WaitGroup
  Log *log.Logger
  workingDir string
  shouldRestart bool
}

func (a *Arbitray) Init() (err error) {
  args := os.Args[1:]
  a.platformInit()

  // Set up our working directory.
  if err = os.Chdir(a.workingDir); err != nil {
    dlgs.Error("Arbitray", err.Error())
    log.Fatalf("Fatal Error: %v", err)
  }

  if (len(args) == 1) {
    if err = os.Chdir(args[0]); err != nil {
      dlgs.Error("Arbitray", err.Error())
      log.Fatalf("Fatal Error: %v", err)
    }
  }

  // Load our config.
  a.config.Load()

  // Set up logging.
  if _, err := os.Stat("logs"); err != nil {
    if os.IsNotExist(err) {
      if err = os.Mkdir("logs", 0755); err != nil {
        dlgs.Error("Arbitray", fmt.Sprintf("What: %d", (err.(*os.PathError)).Err))
        if (err.(*os.PathError)).Err != syscall.EEXIST {
          dlgs.Error("Arbitray", err.Error())
          log.Fatalf("Fatal Error: %v", err)
        }
      }
    }
  }
  // Create Arbitray's log
  a.Log = log.New(nil, "", log.LstdFlags)
  logFile, err := os.OpenFile(fmt.Sprintf("%s/%s.log", "logs", "Arbitray"), os.O_RDWR|os.O_APPEND|os.O_CREATE, 0660)
  if err != nil {
    dlgs.Error("Arbitray", err.Error())
    log.Fatal(err)
  }
  a.Log.SetOutput(logFile)
  return
}

func (a *Arbitray) onReady() {
  if err := arbitray.Init(); err != nil {
    log.Fatal(err)
  }

  systray.SetIcon(iconData)
  systray.SetTitle("")
  systray.SetTooltip("Arbitrary Process Launcher")

  // Add our processes as menu items.
  for _, program := range a.config.Programs {
    program.MenuItem = systray.AddMenuItem(program.Title, program.Tooltip)
    program.CloseChan = make(chan bool)
    program.KillChan = make(chan bool)
    program.Log = log.New(nil, "", log.LstdFlags)
    // This seems heavy to have go routines handling each entry's input...
    go func(program *ArbitrayProgram) {
      for {
        select {
        case <-program.MenuItem.ClickedCh:
          if !program.MenuItem.Checked() {
            a.waitGroup.Add(1)
            go a.startProgram(program)
          } else {
            program.KillChan <- true
          }
        }
      }
    }(program)
  }
  systray.AddSeparator()
  // Add our base items.
  if !a.config.HideItems[CONFIG_STRING] {
    mConfig := systray.AddMenuItem(fmt.Sprintf("✎ %s", CONFIG_STRING), fmt.Sprintf("%s Arbitray", CONFIG_STRING))
    go func() {
      for {
        <-mConfig.ClickedCh
        open("arbitray.json")
      }
    }()
  }

  if !a.config.HideItems[RELOAD_STRING] {
    mReload := systray.AddMenuItem(fmt.Sprintf("↺ %s", RELOAD_STRING), fmt.Sprintf("%s Arbitray (will stop running programs", RELOAD_STRING))
    go func() {
      for {
        <-mReload.ClickedCh
        a.shouldRestart = true
        a.Quit()
      }
    }()
  }

  if !a.config.HideItems[LOGS_STRING] {
    mLogs := systray.AddMenuItem(fmt.Sprintf("☰ %s", LOGS_STRING), fmt.Sprintf("Open Arbitray's %s", LOGS_STRING))
    go func() {
      for {
        <-mLogs.ClickedCh
        openDir("logs")
      }
    }()
  }

  // I guess we'll allow hiding the Quit item.
  if !a.config.HideItems[QUIT_STRING] {
    mQuit := systray.AddMenuItem(fmt.Sprintf("☓ %s", QUIT_STRING), fmt.Sprintf("%s Arbitray", QUIT_STRING))
    go func() {
      for {
        <-mQuit.ClickedCh
        a.Quit()
      }
    }()
  }
}
func (a *Arbitray) onQuit() {
  if a.shouldRestart {
    restart()
  }
}
func (a *Arbitray) Quit() {
  for index, _ := range a.config.Programs {
    if a.config.Programs[index].MenuItem.Checked() == true {
      a.config.Programs[index].KillChan <- true
    }
  }
  a.waitGroup.Wait()
  systray.Quit()
}

func (a *Arbitray) startProgram(p *ArbitrayProgram) {
  defer func() {
    p.MenuItem.Uncheck()
    a.waitGroup.Done()
    fmt.Printf("[Arbitray]: %s finished.\n", p.Title)
    a.Log.Printf("[Arbitray]: %s finished.\n", p.Title)
  }()
  p.MenuItem.Check()

  // Create our loggers.
  logFile, err := os.OpenFile(fmt.Sprintf("%s/%s.log", "logs", p.Title), os.O_RDWR|os.O_APPEND|os.O_CREATE, 0660)
  if err != nil {
    a.Log.Fatal(err)
  }
  defer logFile.Close()
  p.Log.SetOutput(logFile)

  // Set up our command.
  p.CreateCommand()

  var stdinChan, stdoutChan, stderrChan chan string
  // stdin
  if p.Options.CloseCmd != "" {
    stdinChan = make(chan string)
    go func() {
      stdin, err := p.Cmd.StdinPipe()
      if err != nil {
        a.Log.Printf("Uhoh, error getting stdin: %v\n", err)
      }
      for {
        select {
        case out := <-stdinChan:
          io.WriteString(stdin, out)
        }
      }
    }()
  }
  // stdout
  stdoutChan = make(chan string)
  go func() {
    stdout, err := p.Cmd.StdoutPipe()
    if err != nil {
      a.Log.Printf("Uhoh, error getting stdout: %v\n", err)
    }
    reader := bufio.NewReader(stdout)
    for {
      in, err := reader.ReadString('\n')
      if err != nil {
        if err != io.EOF {
          a.Log.Printf("Uhoh, stdout error: %v\n", err)
        }
        return
      }
      stdoutChan <- in
    }
  }()
  // stderr
  stderrChan = make(chan string)
  go func() {
    stderr, err := p.Cmd.StderrPipe()
    if err != nil {
      a.Log.Printf("Uhoh, error getting stderr: %v\n", err)
    }

    reader := bufio.NewReader(stderr)
    for {
      in, err := reader.ReadString('\n')
      if err != nil {
        if err != io.EOF {
          a.Log.Printf("Uhoh, stderr error: %v\n", err)
        }
        return
      }
      stderrChan <- in
    }
  }()
  // Run our command.
  fmt.Printf("[Arbitray]: %s starting.\n", p.Title)
  a.Log.Printf("[Arbitray]: %s starting.\n", p.Title)
  if err := p.Cmd.Start(); err != nil {
    dlgs.Error("Arbitray", err.Error())
    a.Log.Printf("[%s] Error: %s\n", p.Title, err.Error())
    p.Log.Printf("Error: %s\n", err.Error())
  }
  go func() {
    if err := p.Cmd.Wait(); err != nil {
      a.Log.Printf("[%s] Error: %s\n", p.Title, err.Error())
      p.Log.Printf("Error: %s\n", err.Error())
    }
    p.CloseChan <- true
  }()
  //
  ListenLoop:
    for {
      select {
      case msg := <-stdoutChan:
        fmt.Printf("[%s] %s", p.Title, msg)
        p.Log.Println(msg)
      case msg := <-stderrChan:
        fmt.Printf("[%s] %s", p.Title, msg)
        p.Log.Printf("Error: %s", msg)
      case <-p.KillChan:
        if p.Options.CloseCmd != "" {
          stdinChan <- p.Options.CloseCmd
        } else {
          if err := p.Kill(); err != nil {
            dlgs.Error("Arbitray", fmt.Sprintf("Failed to kill process:\n%v", err))
            log.Fatalf("Fatal Error: %v", err)
          }
        }
      case <-p.CloseChan:
        break ListenLoop
      }
    }
}
