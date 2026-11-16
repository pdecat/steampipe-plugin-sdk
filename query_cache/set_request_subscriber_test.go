package query_cache

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/turbot/steampipe-plugin-sdk/v6/grpc"
	sdkproto "github.com/turbot/steampipe-plugin-sdk/v6/grpc/proto"
)

// A reader whose cached page cannot be read (evicted or expired) must release the set
// request's read lock. If it does not, the scan blocks for good on its next row, because
// adding a row needs the write lock.
func TestSetRequestSubscriber_GetRowsToStream_ReleasesLockOnError(t *testing.T) {
	qc, err := NewQueryCache("test", map[string]*grpc.PluginSchema{}, &QueryCacheOptions{Enabled: true, Ttl: 5 * time.Minute, MaxSizeMb: 256})
	if err != nil {
		t.Fatal(err)
	}
	req := newSetRequest(&CacheRequest{Table: "t", CallId: "call", ConnectionName: "conn", resultKeyRoot: "root"}, qc)
	// the scan has written one page, but the page is no longer in the cache
	req.pageCount = 1
	req.rowCount = rowBufferSize

	s := newSetRequestSubscriber(func(*sdkproto.Row) {}, "reader", context.Background(), req)
	if _, err := s.getRowsToStream(); err == nil {
		t.Fatal("getRowsToStream returned no error for a page that is not in the cache")
	}

	if !req.requestLock.TryLock() {
		t.Fatal("getRowsToStream returned an error but left the set request's read lock held")
	}
	req.requestLock.Unlock()
}

// A reader whose execute request is cancelled must not stream a row once it reports done.
// The execute request closes its output channel when its readers are done, so a later row is
// a send on a closed channel: an unrecovered panic that aborts the whole plugin process (in a
// standalone FDW, the postgres backend and every query running in it).
func TestSetRequestSubscriber_StreamsNoRowAfterDone(t *testing.T) {
	ctx := context.Background()
	c, err := NewQueryCache("test", map[string]*grpc.PluginSchema{}, &QueryCacheOptions{Enabled: true, Ttl: time.Hour, MaxSizeMb: 64})
	if err != nil {
		t.Fatal(err)
	}
	req := &CacheRequest{CallId: "scan", Table: "t", Columns: []string{"id"}, Limit: -1, ConnectionName: "conn", TtlSeconds: 300}
	req.resultKeyRoot = c.buildResultKey(req)
	set := newSetRequest(req, c)

	// the scan's own reader keeps the scan going once the other reader has stopped
	own := newSetRequestSubscriber(func(*sdkproto.Row) {}, "own", ctx, set)

	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()
	var done atomic.Bool
	var late atomic.Int32
	reader := newSetRequestSubscriber(func(*sdkproto.Row) {
		if done.Load() {
			late.Add(1)
		}
	}, "reader", streamCtx, set)

	set.requestLock.Lock()
	ownOk, readerOk := set.subscribe(own), set.subscribe(reader)
	set.requestLock.Unlock()
	if !ownOk || !readerOk {
		t.Fatal("could not subscribe to the scan")
	}
	c.setRequestMapLock.Lock()
	c.setRequests[req.CallId] = set
	c.setRequestMapLock.Unlock()

	// the reader waits for rows, as it does while the scan is slow
	reader.readRowsAsync(ctx)
	time.Sleep(20 * time.Millisecond)

	// its execute request is cancelled, and stops accepting rows once the reader is done
	cancelStream()
	<-reader.doneChan
	done.Store(true)

	// the scan goes on, and gives a reader still running time to pick the rows up
	// (it polls every 50ms at most)
	for i := range 10 {
		if err := c.IterateSet(ctx, testRow(i), req.CallId); err != nil {
			t.Fatalf("row %d: IterateSet: %v", i, err)
		}
	}
	time.Sleep(200 * time.Millisecond)

	if n := late.Load(); n > 0 {
		t.Fatalf("the reader streamed %d rows after it reported done", n)
	}
}
