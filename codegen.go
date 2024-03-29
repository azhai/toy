package main

import (
	"fmt"
	"os"

	"tinygo.org/x/go-llvm"
)

var (
	ctx             = llvm.NewContext()
	builder         = ctx.NewBuilder()
	rootModule      = ctx.NewModule("root")
	rootFuncPassMgr = llvm.NewFunctionPassManagerForModule(rootModule)
	options         = llvm.NewMCJITCompilerOptions()
	namedVals       = map[string]llvm.Value{}
	execEngine      llvm.ExecutionEngine
	machine         llvm.TargetMachine
)

func initExecutionEngine() {
	var err error
	var target llvm.Target

	llvm.LinkInMCJIT()

	err = llvm.InitializeNativeTarget()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Native target initialization error:")
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}

	err = llvm.InitializeNativeAsmPrinter()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ASM printer initialization error:")
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}

	target, err = llvm.GetTargetFromTriple(llvm.DefaultTargetTriple())
	if err != nil {
		fmt.Fprintln(os.Stderr, "Cannot get target:")
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}

	if *verbose {
		fmt.Println("Initialize: TargetTriple = " + llvm.DefaultTargetTriple())
		fmt.Println("Initialize: TargetDescription = " + target.Description())
	}

	machine = target.CreateTargetMachine(llvm.DefaultTargetTriple(),
		"", "",
		llvm.CodeGenLevelNone,
		llvm.RelocDefault,
		llvm.CodeModelSmall)
	if *verbose {
		fmt.Println("Initialize: TargetMachine.TargetData = " + machine.CreateTargetData().String())
	}

	options.SetMCJITOptimizationLevel(2)
	options.SetMCJITEnableFastISel(true)
	options.SetMCJITNoFramePointerElim(true)
	options.SetMCJITCodeModel(llvm.CodeModelDefault)
	execEngine, err = llvm.NewMCJITCompiler(rootModule, options)
	if err != nil {
		fmt.Fprintln(os.Stderr, "JIT Compiler initialization error:")
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}

	if *verbose {
		fmt.Println("Initialize: ExecutionEngine.TargetData = " + execEngine.TargetData().String())
	}
}

func Optimize() {
	// rootFuncPassMgr.Add(execEngine.TargetData())
	rootFuncPassMgr.AddPromoteMemoryToRegisterPass()
	rootFuncPassMgr.AddInstructionCombiningPass()
	rootFuncPassMgr.AddReassociatePass()
	rootFuncPassMgr.AddGVNPass()
	rootFuncPassMgr.AddCFGSimplificationPass()
	rootFuncPassMgr.InitializeFunc()
}

func createEntryBlockAlloca(f llvm.Value, name string) llvm.Value {
	var tmpB = llvm.NewBuilder()
	tmpB.SetInsertPoint(f.EntryBasicBlock(), f.EntryBasicBlock().FirstInstruction())
	return tmpB.CreateAlloca(ctx.DoubleType(), name)
}

func (n *fnPrototypeNode) createArgAlloca(f llvm.Value) {
	args := f.Params()
	for i := range args {
		alloca := createEntryBlockAlloca(f, n.args[i])
		builder.CreateStore(args[i], alloca)
		namedVals[n.args[i]] = alloca
	}
}

func (n *numberNode) codegen() llvm.Value {
	return llvm.ConstFloat(ctx.DoubleType(), n.val)
}

func (n *variableNode) codegen() llvm.Value {
	v := namedVals[n.name]
	if v.IsNil() {
		return ErrorV("unknown variable name")
	}
	return builder.CreateLoad(ctx.DoubleType(), v, n.name)
}

func (n *ifNode) codegen() llvm.Value {
	ifv := n.ifN.codegen()
	if ifv.IsNil() {
		return ErrorV("code generation failed for if expression")
	}
	ifv = builder.CreateFCmp(llvm.FloatONE, ifv, llvm.ConstFloat(ctx.DoubleType(), 0), "ifcond")

	parentFunc := builder.GetInsertBlock().Parent()
	thenBlk := llvm.AddBasicBlock(parentFunc, "then")
	elseBlk := llvm.AddBasicBlock(parentFunc, "else")
	mergeBlk := llvm.AddBasicBlock(parentFunc, "merge")
	builder.CreateCondBr(ifv, thenBlk, elseBlk)

	// generate 'then' block
	builder.SetInsertPointAtEnd(thenBlk)
	thenv := n.thenN.codegen()
	if thenv.IsNil() {
		return ErrorV("code generation failed for then expression")
	}
	builder.CreateBr(mergeBlk)
	// Codegen of 'Then' can change the current block, update ThenBB for the PHI.
	thenBlk = builder.GetInsertBlock()

	// generate 'else' block
	// C++ unknown eq: TheFunction->getBasicBlockList().push_back(ElseBB);
	builder.SetInsertPointAtEnd(elseBlk)
	elsev := n.elseN.codegen()
	if elsev.IsNil() {
		return ErrorV("code generation failed for else expression")
	}
	builder.CreateBr(mergeBlk)
	elseBlk = builder.GetInsertBlock()

	builder.SetInsertPointAtEnd(mergeBlk)
	PhiNode := builder.CreatePHI(ctx.DoubleType(), "iftmp")
	PhiNode.AddIncoming([]llvm.Value{thenv}, []llvm.BasicBlock{thenBlk})
	PhiNode.AddIncoming([]llvm.Value{elsev}, []llvm.BasicBlock{elseBlk})
	return PhiNode
}

func (n *forNode) codegen() llvm.Value {
	startVal := n.start.codegen()
	if startVal.IsNil() {
		return ErrorV("code generation failed for start expression")
	}

	parentFunc := builder.GetInsertBlock().Parent()
	alloca := createEntryBlockAlloca(parentFunc, n.counter)
	builder.CreateStore(startVal, alloca)
	loopBlk := llvm.AddBasicBlock(parentFunc, "loop")

	builder.CreateBr(loopBlk)

	builder.SetInsertPointAtEnd(loopBlk)

	// save higher levels' variables if we have the same name
	oldVal := namedVals[n.counter]
	namedVals[n.counter] = alloca

	if n.body.codegen().IsNil() {
		return ErrorV("code generation failed for body expression")
	}

	var stepVal llvm.Value
	if n.step != nil {
		stepVal = n.step.codegen()
		if stepVal.IsNil() {
			return llvm.ConstNull(ctx.DoubleType())
		}
	} else {
		stepVal = llvm.ConstFloat(ctx.DoubleType(), 1)
	}

	// evaluate end condition before increment
	endVal := n.test.codegen()
	if endVal.IsNil() {
		return endVal
	}

	curVar := builder.CreateLoad(ctx.DoubleType(), alloca, n.counter)
	nextVar := builder.CreateFAdd(curVar, stepVal, "nextvar")
	builder.CreateStore(nextVar, alloca)

	endVal = builder.CreateFCmp(llvm.FloatONE, endVal, llvm.ConstFloat(ctx.DoubleType(), 0), "loopcond")
	afterBlk := llvm.AddBasicBlock(parentFunc, "afterloop")

	builder.CreateCondBr(endVal, loopBlk, afterBlk)

	builder.SetInsertPointAtEnd(afterBlk)

	if !oldVal.IsNil() {
		namedVals[n.counter] = oldVal
	} else {
		delete(namedVals, n.counter)
	}

	return llvm.ConstFloat(ctx.DoubleType(), 0)
}

func (n *unaryNode) codegen() llvm.Value {
	operandValue := n.operand.codegen()
	if operandValue.IsNil() {
		return ErrorV("nil operand")
	}

	f := rootModule.NamedFunction("unary" + string(n.name))
	if f.IsNil() {
		return ErrorV("unknown unary operator")
	}
	ftyp := llvm.FunctionType(ctx.DoubleType(), []llvm.Type{ctx.DoubleType()}, false)
	return builder.CreateCall(ftyp, f, []llvm.Value{operandValue}, "unop")
}

func (n *variableExprNode) codegen() llvm.Value {
	var oldvars = []llvm.Value{}

	f := builder.GetInsertBlock().Parent()
	for i := range n.vars {
		name := n.vars[i].name
		node := n.vars[i].node

		var val llvm.Value
		if node != nil {
			val = node.codegen()
			if val.IsNil() {
				return val // nil
			}
		} else { // if no initialized value set to 0
			val = llvm.ConstFloat(ctx.DoubleType(), 0)
		}

		alloca := createEntryBlockAlloca(f, name)
		builder.CreateStore(val, alloca)

		oldvars = append(oldvars, namedVals[name])
		namedVals[name] = alloca
	}

	// evaluate body now that vars are in scope
	bodyVal := n.body.codegen()
	if bodyVal.IsNil() {
		return ErrorV("body returns nil") // nil
	}

	// pop old values
	for i := range n.vars {
		namedVals[n.vars[i].name] = oldvars[i]
	}

	return bodyVal
}

func (n *fnCallNode) codegen() llvm.Value {
	callee := rootModule.NamedFunction(n.callee)
	if callee.IsNil() {
		return ErrorV("unknown function referenced: " + n.callee)
	}

	if callee.ParamsCount() != len(n.args) {
		return ErrorV("incorrect number of arguments passed")
	}

	args, argtyps := []llvm.Value{}, []llvm.Type{}
	for _, arg := range n.args {
		args = append(args, arg.codegen())
		argtyps = append(argtyps, ctx.DoubleType())
		if args[len(args)-1].IsNil() {
			return ErrorV("an argument was nil")
		}
	}

	ftyp := llvm.FunctionType(ctx.DoubleType(), argtyps, false)
	return builder.CreateCall(ftyp, callee, args, "calltmp")
}

func (n *binaryNode) codegen() llvm.Value {
	// Special case '=' because we don't emit the LHS as an expression
	if n.op == "=" {
		l, ok := n.left.(*variableNode)
		if !ok {
			return ErrorV("destination of '=' must be a variable")
		}

		// get value
		val := n.right.codegen()
		if val.IsNil() {
			return ErrorV("cannot assign null value")
		}

		// lookup location of variable from name
		p := namedVals[l.name]

		// store
		builder.CreateStore(val, p)

		return val
	}

	l := n.left.codegen()
	r := n.right.codegen()
	if l.IsNil() || r.IsNil() {
		return ErrorV("operand was nil")
	}

	switch n.op {
	case "+":
		return builder.CreateFAdd(l, r, "addtmp")
	case "-":
		return builder.CreateFSub(l, r, "subtmp")
	case "*":
		return builder.CreateFMul(l, r, "multmp")
	case "/":
		return builder.CreateFDiv(l, r, "divtmp")
	case "<":
		l = builder.CreateFCmp(llvm.FloatOLT, l, r, "cmptmp")
		return builder.CreateUIToFP(l, ctx.DoubleType(), "booltmp")
	default:
		function := rootModule.NamedFunction("binary" + string(n.op))
		if function.IsNil() {
			return ErrorV("invalid binary operator")
		}
		ftyp := llvm.FunctionType(ctx.DoubleType(), []llvm.Type{ctx.DoubleType(), ctx.DoubleType()}, false)
		return builder.CreateCall(ftyp, function, []llvm.Value{l, r}, "binop")
	}
}

func (n *fnPrototypeNode) codegen() llvm.Value {
	funcArgs := []llvm.Type{}
	for range n.args {
		funcArgs = append(funcArgs, ctx.DoubleType())
	}
	funcType := llvm.FunctionType(ctx.DoubleType(), funcArgs, false)
	function := llvm.AddFunction(rootModule, n.name, funcType)

	if function.Name() != n.name {
		function.EraseFromParentAsFunction()
		function = rootModule.NamedFunction(n.name)
	}

	if function.BasicBlocksCount() != 0 {
		return ErrorV("redefinition of function: " + n.name)
	}

	if function.ParamsCount() != len(n.args) {
		return ErrorV("redefinition of function with different number of args")
	}

	for i, param := range function.Params() {
		param.SetName(n.args[i])
		namedVals[n.args[i]] = param
	}

	return function
}

func (n *functionNode) codegen() llvm.Value {
	namedVals = make(map[string]llvm.Value)
	p := n.proto.(*fnPrototypeNode)
	theFunction := n.proto.codegen()
	if theFunction.IsNil() {
		return ErrorV("prototype missing")
	}

	// if p.isOperator && len(p.args) == 2 {
	// 	opChar, _ := utf8.DecodeLastRuneInString(p.name)
	//  binaryOpPrecedence[opChar] = p.precedence
	// }

	block := llvm.AddBasicBlock(theFunction, "entry")
	builder.SetInsertPointAtEnd(block)

	p.createArgAlloca(theFunction)

	retVal := n.body.codegen()
	if retVal.IsNil() {
		theFunction.EraseFromParentAsFunction()
		return ErrorV("function body")
	}

	builder.CreateRet(retVal)
	if llvm.VerifyFunction(theFunction, llvm.PrintMessageAction) != nil {
		theFunction.EraseFromParentAsFunction()
		return ErrorV("function verifiction failed")
	}

	rootFuncPassMgr.RunFunc(theFunction)
	return theFunction
}
