package tools

import (
	"context"
	"errors"
	"reflect"
	"runtime/debug"

	"github.com/robertkrimen/otto"
	_ "github.com/robertkrimen/otto/underscore"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var halt = errors.New("Halt")

type JSCompileArgs struct {
	JS  string `vfilter:"required,field=js,doc=The body of the javascript code."`
	Key string `vfilter:"optional,field=key,doc=If set use this key to cache the JS VM."`
}

func logIfPanic(scope *vfilter.Scope) {
	err := recover()
	if err == halt {
		return
	}

	if err != nil {
		scope.Log("PANIC %v: %v\n", err, string(debug.Stack()))
	}
}

type JSCompile struct{}

func getVM(ctx context.Context,
	scope *vfilter.Scope,
	key string) *otto.Otto {
	if key == "" {
		key = "__jscontext"
	}

	vm, ok := vql_subsystem.CacheGet(scope, key).(*otto.Otto)
	if !ok {
		vm = otto.New()
		vm.Interrupt = make(chan func(), 1)
		go func() {
			<-ctx.Done()
			vm.Interrupt <- func() {
				panic(halt)
			}
		}()
		vql_subsystem.CacheSet(scope, key, vm)
	}

	return vm
}

func (self *JSCompile) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	arg := &JSCompileArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("js: %s", err.Error())
		return vfilter.Null{}
	}

	defer logIfPanic(scope)

	vm := getVM(ctx, scope, arg.Key)
	_, err = vm.Run(arg.JS)
	if err != nil {
		scope.Log("js: %s", err.Error())
		return vfilter.Null{}
	}

	return vfilter.Null{}
}

func (self JSCompile) Info(scope *vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "js",
		Doc:     "Compile and run javascript code.",
		ArgType: type_map.AddType(scope, &JSCompileArgs{}),
	}
}

type JSCallArgs struct {
	Func string      `vfilter:"required,field=func,doc=JS function to call."`
	Args vfilter.Any `vfilter:"required,field=args,doc=Positional args for the function."`
	Key  string      `vfilter:"optional,field=key,doc=If set use this key to cache the JS VM."`
}

type JSCall struct{}

func (self *JSCall) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	arg := &JSCallArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("js_call: %s", err.Error())
		return vfilter.Null{}
	}

	defer logIfPanic(scope)

	var call_args []interface{}
	slice := reflect.ValueOf(arg.Args)

	// A slice of strings.
	if slice.Type().Kind() != reflect.Slice {
		call_args = append(call_args, arg.Args)
	} else {
		for i := 0; i < slice.Len(); i++ {
			value := slice.Index(i).Interface()
			call_args = append(call_args, value)
		}
	}

	vm := getVM(ctx, scope, arg.Key)
	value, err := vm.Call(arg.Func, nil, call_args...)
	if err != nil {
		scope.Log("js_call: %s", err.Error())
		return vfilter.Null{}
	}

	result, _ := value.Export()
	if result == nil {
		result = vfilter.Null{}
	}
	return result
}

func (self JSCall) Info(scope *vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "js_call",
		Doc:     "Compile and run javascript code.",
		ArgType: type_map.AddType(scope, &JSCallArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&JSCall{})
	vql_subsystem.RegisterFunction(&JSCompile{})
}