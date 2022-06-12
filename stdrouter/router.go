package stdrouter

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/makasim/httprouter/radix"
	"github.com/valyala/fasthttp"
)

var HandlerKeyUserValue = "fasthttprouter.handler_id"

type HandlerFunc func(http.ResponseWriter, *http.Request, Params)

// Param is a single URL parameter, consisting of a key and a value.
type Param struct {
	Key   string
	Value string
}

// Params is a Param-slice, as returned by the router.
// The slice is ordered, the first URL parameter is also the first slice value.
// It is therefore safe to read values by the index.
type Params []Param

// ByName returns the value of the first Param which key matches the given name.
// If no matching Param is found, an empty string is returned.
func (ps Params) ByName(name string) string {
	for _, p := range ps {
		if p.Key == name {
			return p.Value
		}
	}
	return ""
}

type Router struct {
	PageNotFoundHandler     http.HandlerFunc
	MethodNotAllowedHandler http.HandlerFunc
	GlobalHandler           HandlerFunc
	Handlers                map[uint64]HandlerFunc

	Trees []radix.Tree

	paramsPool sync.Pool
}

func New() *Router {
	return &Router{
		PageNotFoundHandler: func(rw http.ResponseWriter, _ *http.Request) {
			rw.WriteHeader(http.StatusNotFound)
		},
		MethodNotAllowedHandler: func(rw http.ResponseWriter, _ *http.Request) {
			rw.WriteHeader(http.StatusMethodNotAllowed)
		},
		Handlers: make(map[uint64]HandlerFunc),

		Trees: make([]radix.Tree, 9),
	}
}

func (r *Router) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	i := methodIndexOf(req.Method)
	if i == -1 {
		r.MethodNotAllowedHandler(rw, req)
		return
	}

	ps := r.getParams()
	defer r.putParams(ps)

	hID := r.Trees[i].Search(req.URL.Path, func(n string, v interface{}) {
		v1, ok := v.(string)
		if !ok {
			return // skip
		}

		*ps = append(*ps, Param{
			Key:   n,
			Value: v1,
		})
	})
	if hID == 0 {
		r.PageNotFoundHandler(rw, req)
		return
	}

	*ps = append(*ps, Param{
		Key:   HandlerKeyUserValue,
		Value: strconv.FormatUint(hID, 10),
	})

	if h, ok := r.Handlers[hID]; ok {
		h(rw, req, *ps)
		return
	}

	if r.GlobalHandler != nil {
		r.GlobalHandler(rw, req, *ps)
		return
	}

	r.PageNotFoundHandler(rw, req)
}

func (r *Router) Add(method, path string, handlerID uint64) error {
	methodIndex := methodIndexOf(method)
	if methodIndex == -1 {
		return fmt.Errorf("method not allowed")
	}
	if len(path) == 0 {
		return fmt.Errorf("path empty")
	}

	var err error
	tree := r.Trees[methodIndex].Clone()

	tree, err = tree.Insert(path, handlerID)
	if err != nil {
		return err
	}

	r.Trees[methodIndex] = tree

	return nil
}

func (r *Router) Remove(method, path string) error {
	methodIndex := methodIndexOf(method)
	if methodIndex == -1 {
		return fmt.Errorf("method not allowed")
	}
	if len(path) == 0 {
		return fmt.Errorf("path empty")
	}

	var err error
	tree := r.Trees[methodIndex].Clone()

	tree, err = tree.Delete(path)
	if err != nil {
		return err
	}

	r.Trees[methodIndex] = tree
	return nil
}

func (r *Router) getParams() *Params {
	ps, _ := r.paramsPool.Get().(*Params)
	if ps == nil {
		return &Params{}
	}

	*ps = (*ps)[0:0] // reset slice
	return ps
}

func (r *Router) putParams(ps *Params) {
	if ps != nil {
		r.paramsPool.Put(ps)
	}
}

func methodIndexOf(method string) int {
	switch method {
	case fasthttp.MethodGet:
		return 0
	case fasthttp.MethodHead:
		return 1
	case fasthttp.MethodPost:
		return 2
	case fasthttp.MethodPut:
		return 3
	case fasthttp.MethodPatch:
		return 4
	case fasthttp.MethodDelete:
		return 5
	case fasthttp.MethodConnect:
		return 6
	case fasthttp.MethodOptions:
		return 7
	case fasthttp.MethodTrace:
		return 8
	}

	return -1
}
