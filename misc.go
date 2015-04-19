package main

import (
	"reflect"
)

type mapstrf func(interface{}) string

type mapf func(interface{}) interface{}

type filterf func(interface{}) bool

func IndexList(fn mapstrf, in interface{}) map[string]interface{} {
	return IndexMapList(fn, nil, in)
}

func IndexMapList(fnKey mapstrf, fnVal mapf, in interface{}) map[string]interface{} {
	val := reflect.ValueOf(in)
	out := make(map[string]interface{})

	for i := 0; i < val.Len(); i++ {
		if fnVal != nil {
			out[fnKey(val.Index(i).Interface())] = fnVal(val.Index(i).Interface())
		} else {
			out[fnKey(val.Index(i).Interface())] = val.Index(i).Interface()
		}
	}

	return out
}

func Filter(fn filterf, in interface{}) []interface{} {
	val := reflect.ValueOf(in)
	out := make([]interface{}, 0, val.Len())

	for i := 0; i < val.Len(); i++ {
		current := val.Index(i).Interface()

		if fn(current) {
			out = append(out, current)
		}
	}

	return out
}

func Values(in map[string]interface{}) []interface{} {
	out := make([]interface{}, 0, len(in))
	for _, value := range in {
		out = append(out, value)
	}

	return out
}
