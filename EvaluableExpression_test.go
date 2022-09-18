package govaluate

import (
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
)

var errParam = errors.New("error parmeters")

// 前缀匹配 0:表示整个字符串，1表示需要匹配的字符串，同下
func prefix(args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, errParam
	}

	s, ok := args[0].(string)
	if !ok {
		return nil, errParam
	}
	subStr, ok := args[1].(string)
	if !ok {
		return nil, errParam
	}

	return strings.HasPrefix(s, subStr), nil
}

// 后缀匹配
func suffix(args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, errParam
	}

	s, ok := args[0].(string)
	if !ok {
		return nil, errParam
	}
	subStr, ok := args[1].(string)
	if !ok {
		return nil, errParam
	}

	return strings.HasSuffix(s, subStr), nil
}

var exprFuncs = map[string]ExpressionFunction{
	"prefix": prefix,
	"suffix": suffix,
}

func TestEval(t *testing.T) {
	//expression := "(prefix(key,value, \"xxx\", \"xxxx\") && putTime >= 10) || (putTime <= 100 && prefix(key, \"7/\"))"
	expression := "(prefix(key, \"xxx\") && putTime >= 10) && (putTime <= 100 && prefix(key, \"7/\"))"

	tmpExpr, err := NewEvaluableExpressionWithFunctions(expression, exprFuncs)
	if err != nil {
		t.Fail()
	}

	scope, err := tmpExpr.EvaluateScope(".prefix")
	if err != nil {
		t.Fail()
	}

	if len(scope) != 2 {
		t.Fatalf("scope len must be 2, but now is: %v", len(scope))
	}
	if scope[0].Start != "7/" || scope[1].Start != "xxx" {
		t.Fatalf("scope is invalid")
	}

	expression = "(prefix(key, \"xxx\") && putTime >= 10) || (putTime <= 100)"
	tmpExpr, err = NewEvaluableExpressionWithFunctions(expression, exprFuncs)
	if err != nil {
		t.Fail()
	}

	scope, err = tmpExpr.EvaluateScope(".prefix")
	if err != nil {
		t.Fail()
	}

	if len(scope) != 1 {
		t.Fatalf("scope len must be 1, but now is: %v", len(scope))
	}
	if scope[0].Start != "" {
		t.Fatalf("scope is invalid")
	}

	expression = "(prefix(key, \"xxx\") && putTime >= 10) || putTime <= 100 || prefix(key, \"7/\")"
	tmpExpr, err = NewEvaluableExpressionWithFunctions(expression, exprFuncs)
	if err != nil {
		t.Fail()
	}
	scope, err = tmpExpr.EvaluateScope(".prefix")
	if err != nil {
		t.Fail()
	}
	if len(scope) != 1 {
		t.Fatalf("scope len must be 1, but now is: %v", len(scope))
	}
	if scope[0].Start != "" {
		t.Fatalf("scope is invalid")
	}

	expression = "(prefix(key, \"xxx\") && putTime >= 10) && putTime <= 100 || prefix(key, \"7/\")"
	tmpExpr, err = NewEvaluableExpressionWithFunctions(expression, exprFuncs)
	if err != nil {
		t.Fail()
	}
	scope, err = tmpExpr.EvaluateScope(".prefix")
	if err != nil {
		t.Fail()
	}
	if len(scope) != 2 {
		t.Fatalf("scope len must be 2, but now is: %v", len(scope))
	}
	if scope[0].Start != "7/" || scope[1].Start != "xxx" {
		t.Fatalf("scope is invalid")
	}

	expression = "putTime >= 10 && putTime <= 100"
	tmpExpr, err = NewEvaluableExpressionWithFunctions(expression, exprFuncs)
	if err != nil {
		t.Fail()
	}
	scope, err = tmpExpr.EvaluateScope(".prefix")
	if err != nil {
		t.Fail()
	}
	if len(scope) != 1 {
		t.Fatalf("scope len must be 1, but now is: %v", len(scope))
	}
	if scope[0].Start != "" {
		t.Fatalf("scope is invalid")
	}
}

func TestGetFunction(t *testing.T) {
	expression := "prefix(key, \"xxx\")"
	tmpExpr, err := NewEvaluableExpressionWithFunctions(expression, exprFuncs)
	if err != nil {
		t.Fail()
	}

	info, err := tmpExpr.evaluationStages.GetFunctionInfo()
	if err != nil {
		t.Fatalf("GetFunctionInfo is error:%s", err.Error())
	}

	if !strings.Contains(info.Name, "prefix") {
		t.Fatalf("functionName is invalid:%s", info.Name)
	}

	for _, item := range info.Var {
		if item != "key" {
			t.Fatalf("var is invalid:%s", item)
		}
	}

	for _, item := range info.Params {
		th, ok := item.(string)
		if !ok || th != "xxx" {
			t.Fatalf("params is invalid:%v", item)
		}
	}

	expression = "suffix(key, \"xxx\")"
	tmpExpr, err = NewEvaluableExpressionWithFunctions(expression, exprFuncs)
	if err != nil {
		t.Fail()
	}

	info, err = tmpExpr.evaluationStages.GetFunctionInfo()
	if err != nil {
		t.Fatalf("GetFunctionInfo is error:%s", err.Error())
	}

	if !strings.Contains(info.Name, "suffix") {
		t.Fatalf("functionName is invalid:%s", info.Name)
	}

	for _, item := range info.Var {
		if item != "key" {
			t.Fatalf("var is invalid:%s", item)
		}
	}

	for _, item := range info.Params {
		th, ok := item.(string)
		if !ok || th != "xxx" {
			t.Fatalf("params is invalid:%v", item)
		}
	}

}

func TestStringScope(t *testing.T) {
	a := StringScope{
		Start: "7/",
	}

	b := StringScope{
		Start: "",
	}

	cmp := a.Cmp(&b)
	if cmp != -1 {
		t.Fatalf("7/ is less than empty string")
	}
	cmp = b.Cmp(&a)
	if cmp != 1 {
		t.Fatalf("empty string is greater than 7/")
	}

	b.Start = "7"
	cmp = a.Cmp(&b)
	if cmp != -1 {
		t.Fatalf("7/ is less than 7")
	}
	cmp = b.Cmp(&a)
	if cmp != 1 {
		t.Fatalf("7 is greater than 7/")
	}

	b.Start = "7//"
	cmp = a.Cmp(&b)
	if cmp != 1 {
		t.Fatalf("7// is less than 7/")
	}
	cmp = b.Cmp(&a)
	if cmp != -1 {
		t.Fatalf("7/ is greater than 7//")
	}

	b.Start = "7/"
	cmp = a.Cmp(&b)
	if cmp != 0 {
		t.Fatalf("7/ is equal 7/")
	}
	cmp = b.Cmp(&a)
	if cmp != 0 {
		t.Fatalf("7/ is equal 7/")
	}

	b.Start = "8/"
	cmp = a.Cmp(&b)
	if cmp != 2 {
		t.Fatalf("7/ is Compatible with 8/")
	}
	cmp = b.Cmp(&a)
	if cmp != 2 {
		t.Fatalf("8/ is Compatible with 7/")
	}
}

func TestStringScopes(t *testing.T) {
	scopes := StringScopes{
		&StringScope{
			Start: "",
		},
		&StringScope{
			Start: "7/",
		},
		&StringScope{
			Start: "8/",
		},
		&StringScope{
			Start: "prefix:hello",
		},
		&StringScope{
			Start: "pre",
		},
		&StringScope{
			Start: "hello/",
		},
	}

	if scopes.Len() != 6 {
		t.Fatalf("len is not equal to 6")
	}
	scopes.Swap(0, 1)
	if scopes[0].Start != "7/" {
		t.Fatalf("swap is error")
	}
	if scopes[1].Start != "" {
		t.Fatalf("swap is error")
	}

	cmp := scopes.Less(0, 1)
	if !cmp {
		t.Fatalf("empty string is max")
	}
	cmp = scopes.Less(1, 2)
	if cmp {
		t.Fatalf("empty string is max")
	}

	sort.Sort(scopes)
	if !reflect.DeepEqual(scopes, StringScopes{
		&StringScope{
			Start: "7/",
		},
		&StringScope{
			Start: "8/",
		},
		&StringScope{
			Start: "hello/",
		},
		&StringScope{
			Start: "pre",
		},
		&StringScope{
			Start: "prefix:hello",
		},
		&StringScope{
			Start: "",
		},
	}) {
		t.Fatalf("sort is error")
	}

	scopes = StringScopes{
		&StringScope{
			Start: "7/",
		},
		&StringScope{
			Start: "6/",
		},
		&StringScope{
			Start: "5/",
		},
		&StringScope{
			Start: "",
		},
		&StringScope{
			Start: "4/",
		},
		&StringScope{
			Start: "3/",
		},
	}

	sort.Sort(scopes)
	if !reflect.DeepEqual(scopes, StringScopes{
		&StringScope{
			Start: "3/",
		},
		&StringScope{
			Start: "4/",
		},
		&StringScope{
			Start: "5/",
		},
		&StringScope{
			Start: "6/",
		},
		&StringScope{
			Start: "7/",
		},
		&StringScope{
			Start: "",
		},
	}) {
		t.Fatalf("sort is error")
	}
}

func TestStringScopesMerge(t *testing.T) {
	a := StringScopes{
		&StringScope{
			Start: "",
		},
	}

	b := StringScopes{
		&StringScope{
			Start: "a/",
		},
	}

	tmp := a.Union(b)
	if tmp.Len() != 1 {
		t.Fatalf("len equals to 1")
	}
	if tmp[0].Start != "" {
		t.Fatalf("union equals to empty string")
	}

	tmp = a.Intersection(b)
	if tmp.Len() != 1 {
		t.Fatalf("len equals to 1")
	}

	if tmp[0].Start != "a/" {
		t.Fatalf("inserttion equals to a/")
	}

	a[0].Start = "a//"
	a = append(a, &StringScope{
		Start: "c/",
	})

	a = append(a, &StringScope{
		Start: "d//",
	})

	// a// c/ d//
	// a/
	// union: a/ c/ d//
	// insertion: a// c/ d//

	tmp = a.Union(b)
	if tmp.Len() != 3 {
		t.Fatalf("len equals to 3")
	}

	if tmp[0].Start != "a/" || tmp[1].Start != "c/" || tmp[2].Start != "d//" {
		t.Fatalf("union is error")
	}

	tmp = a.Intersection(b)
	if tmp.Len() != 3 {
		t.Fatalf("len equals to 2")
	}
	if tmp[0].Start != "a//" || tmp[1].Start != "c/" || tmp[2].Start != "d//" {
		t.Fatalf("insertion is error")
	}

	// a// c/ d//
	// ""
	// union: ""
	// insertion: a// c/ d//
	b[0].Start = ""
	tmp = a.Union(b)
	if tmp.Len() != 1 {
		t.Fatalf("len equals to 1")
	}

	if tmp[0].Start != "" {
		t.Fatalf("union is error")
	}

	tmp = a.Intersection(b)
	if tmp.Len() != 3 {
		t.Fatalf("len equals to 3")
	}
	if tmp[0].Start != "a//" || tmp[1].Start != "c/" || tmp[2].Start != "d//" {
		t.Fatalf("insertion is error")
	}

	// a// c/ d//
	// a///
	// union: a// c/ d//
	// insertion: a/// c/ d//
	b[0].Start = "a///"
	tmp = a.Union(b)
	if tmp.Len() != 3 {
		t.Fatalf("len equals to 1")
	}

	if tmp[0].Start != "a//" || tmp[1].Start != "c/" || tmp[2].Start != "d//" {
		t.Fatalf("union is error")
	}

	tmp = a.Intersection(b)
	if tmp.Len() != 3 {
		t.Fatalf("len equals to 3")
	}
	if tmp[0].Start != "a///" || tmp[1].Start != "c/" || tmp[2].Start != "d//" {
		t.Fatalf("insertion is error")
	}

	// a// a/// d//
	// a/
	// union: a/ d//
	// insertion: a///  d//
	a[0].Start = "a//"
	a[1].Start = "a///"
	a[2].Start = "d//"

	b[0].Start = "a/"
	tmp = a.Union(b)
	if tmp.Len() != 2 {
		t.Fatalf("len equals to 2")
	}

	if tmp[0].Start != "a/" || tmp[1].Start != "d//" {
		t.Fatalf("union is error")
	}

	tmp = a.Intersection(b)
	if tmp.Len() != 2 {
		t.Fatalf("len equals to 2")
	}
	if tmp[0].Start != "a///" || tmp[1].Start != "d//" {
		t.Fatalf("insertion is error")
	}
}
