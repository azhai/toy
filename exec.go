package main

import (
	"fmt"
	"os"

	"tinygo.org/x/go-llvm"
)

func VisitNodes(roots <-chan node, action func(node, llvm.Value)) {
	for nod := range roots {
		val := nod.codegen()
		if val.IsNil() {
			fmt.Fprintln(os.Stderr, "Error: Codegen failed; skipping.")
			continue
		}
		if action != nil {
			action(nod, val)
		}
	}
}

func Compile(roots <-chan node, module llvm.Module) ([]byte, error) {
	VisitNodes(roots, nil)
	buffer, err := machine.EmitToMemoryBuffer(module, llvm.ObjectFile)
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func EmitIR(roots <-chan node) {
	fmt.Fprintln(os.Stdout, rootModule.String())
	VisitNodes(roots, func(nod node, val llvm.Value) {
		val.Dump()
	})
}

// Exec JIT-compiles the top level statements in the roots chan and,
// if they are expressions, executes them.
func Exec(roots <-chan node) {
	VisitNodes(roots, func(nod node, val llvm.Value) {
		if isTopLevelExpr(nod) {
			returnval := execEngine.RunFunction(val, []llvm.GenericValue{})
			fmt.Printf("Evaluated to: %v\n", returnval.Float(llvm.DoubleType()))
		}
	})
}

// getFuncName determines if the node is function and its name.
func getFuncName(n node) (bool, string) {
	if n.Kind() != nodeFunction {
		return false, ""
	}
	name := n.(*functionNode).proto.(*fnPrototypeNode).name
	return true, name
}

// isTopLevelExpr determines if the node is a top level expression.
// Top level expressions are function nodes with no name.
func isTopLevelExpr(n node) bool {
	isFunc, name := getFuncName(n)
	return isFunc && (name == "" || name == "main")
}
