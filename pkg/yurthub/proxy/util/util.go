package util

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/alibaba/openyurt/pkg/yurthub/util"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/klog"
)

const (
	canCacheHeader string = "Edge-Cache"
)

// WithRequestContentType add req-content-type in request context.
// if no Accept header is set, application/vnd.kubernetes.protobuf will be used
func WithRequestContentType(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		if info, ok := apirequest.RequestInfoFrom(ctx); ok {
			if info.IsResourceRequest {
				var contentType string
				header := req.Header.Get("Accept")
				parts := strings.Split(header, ",")
				if len(parts) >= 1 {
					contentType = parts[0]
				}

				if len(contentType) == 0 {
					klog.Errorf("no accept content type for request: %s", util.ReqString(req))
					http.Error(w, "no accept content type is set.", http.StatusBadRequest)
					return
				}

				ctx = util.WithReqContentType(ctx, contentType)
				req = req.WithContext(ctx)
			}
		}

		handler.ServeHTTP(w, req)
	})
}

// WithCacheHeaderCheck add cache agent for response cache
// in default mode, only kubelet, kube-proxy, flanneld, coredns User-Agent
// can be supported to cache response. and with Edge-Cache header is also supported.
func WithCacheHeaderCheck(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		if info, ok := apirequest.RequestInfoFrom(ctx); ok {
			if info.IsResourceRequest {
				needToCache := strings.ToLower(req.Header.Get(canCacheHeader))
				if needToCache == "true" {
					ctx = util.WithReqCanCache(ctx, true)
					req = req.WithContext(ctx)
				}
				req.Header.Del(canCacheHeader)
			}
		}

		handler.ServeHTTP(w, req)
	})
}

// WithRequestClientComponent add component field in request context.
// component is extracted from User-Agent Header, and only the content
// before the "/" when User-Agent include "/".
func WithRequestClientComponent(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		if info, ok := apirequest.RequestInfoFrom(ctx); ok {
			if info.IsResourceRequest {
				var comp string
				userAgent := strings.ToLower(req.Header.Get("User-Agent"))
				parts := strings.Split(userAgent, "/")
				if len(parts) > 0 {
					comp = strings.ToLower(parts[0])
				}

				if comp != "" {
					ctx = util.WithClientComponent(ctx, comp)
					req = req.WithContext(ctx)
				}
			}
		}

		handler.ServeHTTP(w, req)
	})
}

type wrapperResponseWriter struct {
	http.ResponseWriter
	statusCode    int
	closeNotifyCh chan bool
	ctx           context.Context
}

func newWrapperResponseWriter(ctx context.Context, rw http.ResponseWriter) *wrapperResponseWriter {
	return &wrapperResponseWriter{
		ResponseWriter: rw,
		closeNotifyCh:  make(chan bool, 1),
		ctx:            ctx,
	}
}

func (wrw *wrapperResponseWriter) Write(b []byte) (int, error) {
	return wrw.ResponseWriter.Write(b)
}

func (wrw *wrapperResponseWriter) Header() http.Header {
	return wrw.ResponseWriter.Header()
}

func (wrw *wrapperResponseWriter) WriteHeader(statusCode int) {
	wrw.statusCode = statusCode
	wrw.ResponseWriter.WriteHeader(statusCode)
}

func (wrw *wrapperResponseWriter) CloseNotify() <-chan bool {
	if cn, ok := wrw.ResponseWriter.(http.CloseNotifier); ok {
		return cn.CloseNotify()
	}
	klog.Infof("can't get http.CloseNotifier from http.ResponseWriter")
	go func() {
		select {
		case <-wrw.ctx.Done():
			wrw.closeNotifyCh <- true
		}
	}()

	return wrw.closeNotifyCh
}

func (wrw *wrapperResponseWriter) Flush() {
	if flusher, ok := wrw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	} else {
		klog.Errorf("can't get http.Flusher from http.ResponseWriter")
	}
}

// WithRequestTrace used for tracing in flight requests. and when in flight
// requests exceeds the threshold, the following incoming requests will be rejected.
func WithRequestTrace(handler http.Handler, limit int) http.Handler {
	var reqChan chan bool
	if limit > 0 {
		reqChan = make(chan bool, limit)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		wrapperRW := newWrapperResponseWriter(req.Context(), w)
		start := time.Now()

		select {
		case reqChan <- true:
			defer func() {
				<-reqChan
				klog.Infof("%s with status code %d, spent %v, left %d requests in flight", util.ReqString(req), wrapperRW.statusCode, time.Now().Sub(start), len(reqChan))
			}()
			handler.ServeHTTP(wrapperRW, req)
		default:
			// Return a 429 status indicating "Too Many Requests"
			w.Header().Set("Retry-After", "1")
			http.Error(w, "Too many requests, please try again later.", http.StatusTooManyRequests)
		}
	})
}
