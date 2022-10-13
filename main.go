package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"tinygo.org/x/go-llvm"
)

var (
	dumpIR   = flag.Bool("d", false, "dump the llvm ir")
	execProg = flag.Bool("e", false, "evaluate the code")
	optimize = flag.Int("O", 0, "the level of optimization")
	output   = flag.String("o", "", "output filename")
	verbose  = flag.Bool("v", false, "verbose output")
	writer   *os.File
	err      error
)

func main() {
	flag.Parse()
	files := flag.Args()
	if len(files) == 0 {
		return
	}
	initExecutionEngine()
	if *optimize > 0 {
		Optimize()
	}

	needWrite, filename, extname := true, "", ""
	if *output == "" {
		if *dumpIR || *execProg {
			needWrite = false
		} else {
			filename = filepath.Base(files[0])
			pos := len(filename) - len(filepath.Ext(filename))
			*output = filename[:pos]
		}
	}
	filename = *output
	if needWrite {
		extname = strings.ToLower(filepath.Ext(*output))
		if extname == "" {
			filename += ".o"
		}
		writer, err = os.Create(filename)
		handleError(true, "can not open the file:", err)
		defer writer.Close()
	}

	lex := Lex()
	go func() {
		for _, fn := range files {
			f, err := os.Open(fn)
			handleError(true, "", err)
			lex.Add(f)
		}
		lex.Done()
	}()

	tokens := lex.Tokens()
	if extname == ".tok" {
		for tok := range tokens {
			spew.Fdump(writer, tok)
		}
		if needWrite {
			return
		}
	}

	nodes := Parse(tokens)
	if *dumpIR {
		EmitIR(nodes)
	}
	if *execProg {
		Exec(nodes)
	}
	if !needWrite {
		return
	}

	switch extname {
	case ".ast":
		for nod := range nodes {
			spew.Fdump(writer, nod)
		}
	case ".bc":
		VisitNodes(nodes, nil)
		llvm.WriteBitcodeToFile(rootModule, writer)
	default:
		obj, err := Compile(nodes, rootModule)
		handleError(true, "can not emit object file to memory buffer:", err)
		_, err = writer.Write(obj)
		handleError(true, "write to file failure:", err)
		if extname == "" {
			cmd := exec.Command("clang", "-o", *output, "lib.c", filename)
			// fmt.Println(cmd.String())
			handleError(true, "build failure:", cmd.Run())
			os.Chmod(*output, 0755)
		}
	}
}

func handleError(isExit bool, msg string, err error) {
	if err == nil {
		return
	}
	if len(msg) > 0 {
		fmt.Println(msg)
	}
	fmt.Println(err)
	if isExit {
		os.Exit(-1)
	}
}
