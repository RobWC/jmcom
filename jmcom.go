package main

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"sync"

	log "github.com/Sirupsen/logrus"
	"github.com/kardianos/osext"
)

//Tool globals
var msgChannel chan Message
var ctrlChans map[string]chan Message
var logFiles map[string]*os.File
var hosts string
var user string
var password string
var commands string
var commandFile string
var logs bool
var logLocation string
var hostsFile string
var commandWg sync.WaitGroup
var connectWg sync.WaitGroup
var recWg sync.WaitGroup

func init() {
	//TODO: Add CSV and SSH key support

	//Define flags for calling script
	flag.StringVar(&hosts, "hosts", "", "Define hosts to connect to: 1.2.3.3 or 2.3.4.5,1.2.3.4")
	flag.StringVar(&user, "user", "", "Specify the username to use against hosts")
	flag.StringVar(&password, "password", "", "Specify password to use with hosts")
	flag.StringVar(&commands, "command", "", "Commands to run against host: \"show version\" or for multiple commands \"show version\",\"show chassis hardware\"")
	flag.StringVar(&hostsFile, "hosts-file", "", "File to load hosts from")
	flag.StringVar(&commandFile, "cmd-file", "", "File to load commands from")
	flag.BoolVar(&logs, "log", false, "Log output for each host to a seperate file")
	flag.StringVar(&logLocation, "logdir", "", "Directory to write logs to. Default is current directory")
}

func main() {
	flag.Parse()

	//create channels for communication
	msgChannel = make(chan Message)
	ctrlChans = make(map[string]chan Message)
	//Create map for logging
	logFiles = make(map[string]*os.File)

	//Split hosts
	hs := strings.Split(hosts, ",")

	//setup log files
	if logs {
		for _, v := range hs {
			var err error
			logFiles[v], err = OpenLog(logLocation, v)
			if err != nil {
				log.Fatalln(err)
			}
		}
	}

	recWg.Add(1)
	go func() {
		for {
			select {
			case msg, chanOpen := <-msgChannel:
				if chanOpen && msg.Error != nil {
					log.Errorf("Session %d error: %s", msg.SessionID, msg.Error)
				} else if chanOpen && msg.Data != "" && msg.Host != "" {
					if logs {
						log.SetOutput(logFiles[msg.Host])
						log.Printf("Host: %s SessionID: %d Command: %s\n%s", msg.Host, msg.SessionID, msg.Command, msg.Data)
						log.SetOutput(os.Stdout)
					} else {
						log.Printf("Host: %s SessionID: %d Command: %s\n%s", msg.Host, msg.SessionID, msg.Command, msg.Data)
					}
					commandWg.Done()
				} else {
					recWg.Done()
					return
				}
			}
		}
	}()

	if hosts != "" && user != "" && password != "" {
		for _, v := range hs {
			ctrlChans[v] = make(chan Message)
			connectWg.Add(1)
			a := &Agent{Username: user, Password: password, Host: v, connectWg: connectWg, CtrlChannel: ctrlChans[v], MsgChannel: msgChannel}
			log.Println("Connecting to", v)
			go a.Run()
		}
	}
	connectWg.Wait()
	//Run command against hosts
	cmds := strings.Split(commands, ",")
	for _, v := range cmds {
		for item := range ctrlChans {
			commandWg.Add(1)
			ctrlChans[item] <- Message{Command: v}
		}
	}

	//return results
	commandWg.Wait()
	close(msgChannel)
	for item := range ctrlChans {
		close(ctrlChans[item])
	}
	recWg.Wait()
	log.Println("Tasks Complete")
}

//OpenLog open log file for writing
func OpenLog(path string, filename string) (*os.File, error) {
	if path == "" {
		//use current directory
		curdir, err := osext.ExecutableFolder()
		if err != nil {
			log.Fatalln(err)
		}
		dir, err := filepath.Abs(filepath.Dir(strings.Join([]string{curdir, filename}, "/")))
		if err != nil {
			log.Fatal(err)
		}
		return os.OpenFile(strings.Join([]string{dir, strings.Join([]string{filename, ".log"}, "")}, "/"), os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660)
	}
	return &os.File{}, errors.New("Unable to determine path for writing")
}