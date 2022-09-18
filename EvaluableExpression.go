package govaluate

import (
	"errors"
	"fmt"
	"log"
	"reflect"
	"runtime"
	"sort"
	"strings"
)

const isoDateFormat string = "2006-01-02T15:04:05.999999999Z0700"
const shortCircuitHolder int = -1

var DUMMY_PARAMETERS = MapParameters(map[string]interface{}{})

var InValidError = errors.New("invalid expression")

/*
EvaluableExpression represents a set of ExpressionTokens which, taken together,
are an expression that can be evaluated down into a single value.
*/
type EvaluableExpression struct {

	/*
		Represents the query format used to output dates. Typically only used when creating SQL or Mongo queries from an expression.
		Defaults to the complete ISO8601 format, including nanoseconds.
	*/
	QueryDateFormat string

	/*
		Whether or not to safely check types when evaluating.
		If true, this library will return error messages when invalid types are used.
		If false, the library will panic when operators encounter types they can't use.

		This is exclusively for users who need to squeeze every ounce of speed out of the library as they can,
		and you should only set this to false if you know exactly what you're doing.
	*/
	ChecksTypes bool

	tokens           []ExpressionToken
	evaluationStages *evaluationStage
	inputExpression  string
}

/*
Parses a new EvaluableExpression from the given [expression] string.
Returns an error if the given expression has invalid syntax.
*/
func NewEvaluableExpression(expression string) (*EvaluableExpression, error) {

	functions := make(map[string]ExpressionFunction)
	return NewEvaluableExpressionWithFunctions(expression, functions)
}

/*
Similar to [NewEvaluableExpression], except that instead of a string, an already-tokenized expression is given.
This is useful in cases where you may be generating an expression automatically, or using some other parser (e.g., to parse from a query language)
*/
func NewEvaluableExpressionFromTokens(tokens []ExpressionToken) (*EvaluableExpression, error) {

	var ret *EvaluableExpression
	var err error

	ret = new(EvaluableExpression)
	ret.QueryDateFormat = isoDateFormat

	err = checkBalance(tokens)
	if err != nil {
		return nil, err
	}

	err = checkExpressionSyntax(tokens)
	if err != nil {
		return nil, err
	}

	ret.tokens, err = optimizeTokens(tokens)
	if err != nil {
		return nil, err
	}

	ret.evaluationStages, err = planStages(ret.tokens)
	if err != nil {
		return nil, err
	}

	ret.ChecksTypes = true
	return ret, nil
}

/*
Similar to [NewEvaluableExpression], except enables the use of user-defined functions.
Functions passed into this will be available to the expression.
*/
func NewEvaluableExpressionWithFunctions(expression string, functions map[string]ExpressionFunction) (*EvaluableExpression, error) {

	var ret *EvaluableExpression
	var err error

	ret = new(EvaluableExpression)
	ret.QueryDateFormat = isoDateFormat
	ret.inputExpression = expression

	ret.tokens, err = parseTokens(expression, functions)
	if err != nil {
		return nil, err
	}

	err = checkBalance(ret.tokens)
	if err != nil {
		return nil, err
	}

	err = checkExpressionSyntax(ret.tokens)
	if err != nil {
		return nil, err
	}

	ret.tokens, err = optimizeTokens(ret.tokens)
	if err != nil {
		return nil, err
	}

	ret.evaluationStages, err = planStages(ret.tokens)
	if err != nil {
		return nil, err
	}

	ret.ChecksTypes = true
	return ret, nil
}

/*
Same as `Eval`, but automatically wraps a map of parameters into a `govalute.Parameters` structure.
*/
func (this EvaluableExpression) Evaluate(parameters map[string]interface{}) (interface{}, error) {

	if parameters == nil {
		return this.Eval(nil)
	}
	return this.Eval(MapParameters(parameters))
}

/*
Runs the entire expression using the given [parameters].
e.g., If the expression contains a reference to the variable "foo", it will be taken from `parameters.Get("foo")`.

This function returns errors if the combination of expression and parameters cannot be run,
such as if a variable in the expression is not present in [parameters].

In all non-error circumstances, this returns the single value result of the expression and parameters given.
e.g., if the expression is "1 + 1", this will return 2.0.
e.g., if the expression is "foo + 1" and parameters contains "foo" = 2, this will return 3.0
*/
func (this EvaluableExpression) Eval(parameters Parameters) (interface{}, error) {

	if this.evaluationStages == nil {
		return nil, nil
	}

	if parameters != nil {
		parameters = &sanitizedParameters{parameters}
	}
	return this.evaluateStage(this.evaluationStages, parameters)
}

func (this EvaluableExpression) evaluateStage(stage *evaluationStage, parameters Parameters) (interface{}, error) {

	var left, right interface{}
	var err error

	if stage.leftStage != nil {
		left, err = this.evaluateStage(stage.leftStage, parameters)
		if err != nil {
			return nil, err
		}
	}

	if stage.isShortCircuitable() {
		switch stage.symbol {
		case AND:
			if left == false {
				return false, nil
			}
		case OR:
			if left == true {
				return true, nil
			}
		case COALESCE:
			if left != nil {
				return left, nil
			}

		case TERNARY_TRUE:
			if left == false {
				right = shortCircuitHolder
			}
		case TERNARY_FALSE:
			if left != nil {
				right = shortCircuitHolder
			}
		}
	}

	if right != shortCircuitHolder && stage.rightStage != nil {
		right, err = this.evaluateStage(stage.rightStage, parameters)
		if err != nil {
			return nil, err
		}
	}

	if this.ChecksTypes {
		if stage.typeCheck == nil {

			err = typeCheck(stage.leftTypeCheck, left, stage.symbol, stage.typeErrorFormat)
			if err != nil {
				return nil, err
			}

			err = typeCheck(stage.rightTypeCheck, right, stage.symbol, stage.typeErrorFormat)
			if err != nil {
				return nil, err
			}
		} else {
			// special case where the type check needs to know both sides to determine if the operator can handle it
			if !stage.typeCheck(left, right) {
				errorMsg := fmt.Sprintf(stage.typeErrorFormat, left, stage.symbol.String())
				return nil, errors.New(errorMsg)
			}
		}
	}

	return stage.operator(left, right, parameters)
}

var MaxScope = StringScope{
	Start: "",
}

type StringScope struct {
	Start string
}

/**
 	* -1: less
 	* 0:equal
	* 1: great
	* 2: 无法比较
*/
func (left *StringScope) Cmp(right *StringScope) int {
	if right == nil {
		panic("right must be not nil")
	}

	if left.Start == right.Start {
		return 0
	}

	if left.Start == "" {
		return 1
	}

	if right.Start == "" {
		return -1
	}

	if strings.HasPrefix(left.Start, right.Start) {
		return -1
	}

	if strings.HasPrefix(right.Start, left.Start) {
		return 1
	}

	return 2
}

type StringScopes []*StringScope

func (t StringScopes) Len() int {
	return len(t)
}

func (t StringScopes) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
}

// 按照字母排序
func (t StringScopes) Less(i, j int) bool {
	if t[i].Start == "" {
		return false
	}

	if t[j].Start == "" {
		return true
	}

	return t[i].Start < t[j].Start
}

func (t StringScopes) Union(rhs StringScopes) StringScopes {
	tLen := t.Len()
	rLen := rhs.Len()
	union := make(StringScopes, 0)

	sort.Sort(t)
	sort.Sort(rhs)

	tStart := 0
	rStart := 0

	for {
		if tStart >= tLen {
			break
		}

		if rStart >= rLen {
			break
		}

		if t[tStart].Start == "" || rhs[rStart].Start == "" {
			return []*StringScope{&MaxScope}
		}

		res := t[tStart].Start < rhs[rStart].Start
		var dest StringScopes = nil
		idx := 0
		if res {
			dest = t
			idx = tStart
			tStart++
		} else {
			dest = rhs
			idx = rStart
			rStart++
		}

		if len(union) == 0 {
			union = append(union, dest[idx])
			continue
		}

		tmp := union[len(union)-1].Cmp(dest[idx])
		if tmp == 2 {
			union = append(union, dest[idx])
		} else if tmp == -1 {
			union[len(union)-1] = dest[idx]
		}
	}

	var needProcess StringScopes = nil
	needProcessLen := -1
	needProcessIdx := -1

	if tStart < tLen {
		needProcess = t
		needProcessLen = tLen
		needProcessIdx = tStart
	}

	if rStart < rLen {
		needProcess = rhs
		needProcessLen = rLen
		needProcessIdx = rStart
	}

	if needProcessIdx != -1 && needProcessLen != -1 {
		for ; needProcessIdx < needProcessLen; needProcessIdx++ {
			if len(union) == 0 {
				union = append(union, needProcess[needProcessIdx])
				continue
			}

			tmp := union[len(union)-1].Cmp(needProcess[needProcessIdx])
			if tmp == 2 {
				union = append(union, needProcess[needProcessIdx])
			} else if tmp == -1 {
				union[len(union)-1] = needProcess[needProcessIdx]
			}
		}
	}
	return union
}

func (t StringScopes) Intersection(rhs StringScopes) StringScopes {
	tLen := t.Len()
	rLen := rhs.Len()

	sort.Sort(t)
	sort.Sort(rhs)

	union := make(StringScopes, 0)

	//默认放入最大的范围
	union = append(union, &MaxScope)

	tStart := 0
	rStart := 0

	for {
		if tStart >= tLen {
			break
		}

		if rStart >= rLen {
			break
		}

		if t[tStart].Start == "" {
			tStart++
			continue
		}

		if rhs[rStart].Start == "" {
			rStart++
			continue
		}

		//字符串的比较
		res := t[tStart].Start < rhs[rStart].Start
		var dest StringScopes = nil
		idx := 0
		if res {
			dest = t
			idx = tStart
			tStart++
		} else {
			dest = rhs
			idx = rStart
			rStart++
		}

		// union最后一个数进行比较
		tmp := union[len(union)-1].Cmp(dest[idx])
		if tmp == 2 {
			union = append(union, dest[idx])
		} else if tmp == 1 {
			union[len(union)-1] = dest[idx]
		}
	}

	var needProcess StringScopes = nil
	needProcessLen := -1
	needProcessIdx := -1

	if tStart < tLen {
		needProcess = t
		needProcessLen = tLen
		needProcessIdx = tStart
	}

	if rStart < rLen {
		needProcess = rhs
		needProcessLen = rLen
		needProcessIdx = rStart
	}

	if needProcessIdx != -1 && needProcessLen != -1 {
		for ; needProcessIdx < needProcessLen; needProcessIdx++ {
			tmp := union[len(union)-1].Cmp(needProcess[needProcessIdx])
			if tmp == 2 {
				union = append(union, needProcess[needProcessIdx])
			} else if tmp == 1 {
				union[len(union)-1] = needProcess[needProcessIdx]
			}
		}
	}

	return union
}

func (this EvaluableExpression) EvaluateScope(funcName string) (StringScopes, error) {
	return this.evalScopeStage(this.evaluationStages, funcName)
}

func (this EvaluableExpression) evalScopeStage(stage *evaluationStage, functionName string) (StringScopes, error) {
	var left, right StringScopes
	var err error

	if stage.leftStage != nil {
		left, err = this.evalScopeStage(stage.leftStage, functionName)
		if err != nil {
			return nil, err
		}
	}

	if stage.rightStage != nil {
		right, err = this.evalScopeStage(stage.rightStage, functionName)
		if err != nil {
			return nil, err
		}
	}

	switch stage.symbol {
	case AND:
		{
			if left == nil || right == nil {
				log.Printf("token<AND>:%++v eval scope is error", stage.token)
				return nil, errors.New("eval scope is error")
			}

			return left.Intersection(right), nil
		}
	case OR:
		{
			if left == nil || right == nil {
				log.Printf("token<OR>:%++v eval scope is error", stage.token)
				return nil, errors.New("eval scope is error")
			}
			return left.Union(right), nil
		}
	case NOOP:
		{
			if stage.rightStage != nil {
				return this.evalScopeStage(stage.rightStage, functionName)
			} else {
				log.Printf("token<NOOP>:%++v eval scope is error", stage.token)
				return nil, InValidError
			}
		}
	case FUNCTIONAL:
		{
			if stage.token.Kind != FUNCTION {
				log.Printf("token<FUNCTIONAL>:%++v eval scope is error", stage.token)
				return nil, InValidError
			}

			funcName := runtime.FuncForPC(reflect.ValueOf(stage.token.Value).Pointer()).Name()
			if !strings.Contains(funcName, functionName) {
				return []*StringScope{&MaxScope}, nil
			}

			info, err := stage.GetFunctionInfo()
			if err != nil {
				return nil, err
			}
			for i, item := range info.Var {
				if item == "key" {
					tmpKey, ok := info.Params[i].(string)
					if !ok {
						log.Printf("key parameter is not a string")
						return nil, InValidError
					}
					return []*StringScope{
						&StringScope{
							Start: tmpKey,
						},
					}, nil
				}
			}
		}
	}
	return []*StringScope{&MaxScope}, nil
}

func typeCheck(check stageTypeCheck, value interface{}, symbol OperatorSymbol, format string) error {

	if check == nil {
		return nil
	}

	if check(value) {
		return nil
	}

	errorMsg := fmt.Sprintf(format, value, symbol.String())
	return errors.New(errorMsg)
}

/*
Returns an array representing the ExpressionTokens that make up this expression.
*/
func (this EvaluableExpression) Tokens() []ExpressionToken {

	return this.tokens
}

/*
Returns the original expression used to create this EvaluableExpression.
*/
func (this EvaluableExpression) String() string {

	return this.inputExpression
}

/*
Returns an array representing the variables contained in this EvaluableExpression.
*/
func (this EvaluableExpression) Vars() []string {
	var varlist []string
	for _, val := range this.Tokens() {
		if val.Kind == VARIABLE {
			varlist = append(varlist, val.Value.(string))
		}
	}
	return varlist
}
