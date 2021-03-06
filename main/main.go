package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Azure/azure-docker-extension/pkg/vmextension/status"

	"github.com/Azure/azure-docker-extension/pkg/vmextension"
	"github.com/go-kit/kit/log"
	"strconv"
)

// flags for debugging and printing detailed reports
type flags struct {
	verbose bool
	debug   bool
}

var (
	verbose = flag.Bool("verbose", false, "Return a detailed report")
	debug   = flag.Bool("debug", false, "Return a debug report")

	// the logger that will be used throughout
	lg ExtensionLogger

	// this logger is used only for testing purposes
	noopLogger = ExtensionLogger{log.NewNopLogger(), ""}
)

func main() {
	// parse extension environment
	hEnv, handlerErr := vmextension.GetHandlerEnv()
	if handlerErr != nil {
		lg.eventError("failed to parse handlerEnv", handlerErr)
		os.Exit(failureCode)
	}

	lg = newLogger(hEnv.HandlerEnvironment.LogFolder)

	// parse the command line arguments
	flag.Parse()
	cmd := parseCmd(flag.Args())
	lg.with("Operation: ", cmd.name)
	lg.customLog("Command: ", cmd.name)

	seqNum, seqErr := vmextension.FindSeqNum(hEnv.HandlerEnvironment.ConfigFolder)
	if seqErr != nil {
		lg.eventError("failed to find sequence number", seqErr)
		// only throw a fatal error if the command is not install
		if cmd.name != "install" {
			os.Exit(cmd.failExitCode)
		}
	}
	lg.event("seqNum: " + strconv.Itoa(seqNum))

	// check sub-command preconditions, if any, before executing
	lg.event("start operation")
	if cmd.pre != nil {
		lg.event("pre-check")
		if preErr := cmd.pre(lg, seqNum); preErr != nil {
			lg.eventError("pre-check failed", preErr)
			telemetry(TelemetryScenario, "enable pre-check failed: "+preErr.Error(), false, 0)
			os.Exit(cmd.failExitCode)
		}
	}

	// execute the command
	lg.event("reporting status")
	reportStatus(lg, hEnv, seqNum, status.StatusTransitioning, cmd, "Transitioning")

	if cmdErr := cmd.f(lg, hEnv, seqNum); cmdErr != nil {
		lg.eventError("command failed", cmdErr)
		reportStatus(lg, hEnv, seqNum, status.StatusError, cmd, cmdErr.Error())
		telemetry(TelemetryScenario, cmd.name+" failed: "+cmdErr.Error(), false, 0)
		os.Exit(cmd.failExitCode)
	}
	reportStatus(lg, hEnv, seqNum, status.StatusSuccess, cmd, "")

	telemetry(TelemetryScenario, cmd.name+" succeeded", false, 0)
	lg.event(cmd.name + " end")

	os.Exit(successCode)
}

// parseCmd looks at the input array and parses the subcommand. If it is invalid,
// it prints the usage string and an error message and exits with code 2.
func parseCmd(args []string) cmd {
	if len(args) != 1 {
		if len(args) < 1 {
			fmt.Printf("Not enough arguments, %d", len(args))
			fmt.Println()
			fmt.Printf("%v", args)
			fmt.Println()
		} else {
			fmt.Println("Too many arguments")
		}
		printUsage(args)
		os.Exit(invalidCmdCode)
	}
	// ensure arguments passed are all lower case
	cmd, ok := cmds[strings.ToLower(args[0])]
	if !ok {
		printUsage(args)
		fmt.Printf("Incorrect command: %q\n", args[0])
		os.Exit(invalidCmdCode)
	}
	return cmd
}

// printUsage prints the help string and version of the program to stdout with a
// trailing new line.
func printUsage(args []string) {
	fmt.Printf("Usage: %s ", "main.exe")
	i := 0
	for k := range cmds {
		fmt.Print(k)
		if i != len(cmds)-1 {
			fmt.Printf(" | ")
		}
		i++
	}
	fmt.Println()

	fmt.Println("Optional flags: verbose | debug")
}
