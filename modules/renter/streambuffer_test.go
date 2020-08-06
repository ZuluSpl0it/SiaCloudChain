package renter

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"sync"
	"testing"
	"time"

	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/NebulousLabs/threadgroup"

	"gitlab.com/scpcorp/ScPrime/crypto"
)

// mockDataSource implements a stream buffer data source that can be used to
// test the stream buffer. It's a simple in-memory buffer.
type mockDataSource struct {
	data              []byte
	staticRequestSize uint64
	mu                sync.Mutex
}

// newMockDataSource will return a data source that is ready to use.
func newMockDataSource(data []byte, requestSize uint64) *mockDataSource {
	return &mockDataSource{
		data:              data,
		staticRequestSize: requestSize,
	}
}

// DataSize implements streamBufferDataSource
func (mds *mockDataSource) DataSize() uint64 {
	mds.mu.Lock()
	defer mds.mu.Unlock()
	return uint64(len(mds.data))
}

// ID implements streamBufferDataSource
func (mds *mockDataSource) ID() streamDataSourceID {
	return streamDataSourceID(crypto.HashAll(mds))
}

// RequestSize implements streamBufferDataSource.
func (mds *mockDataSource) RequestSize() uint64 {
	return mds.staticRequestSize
}

// ReadAt implements streamBufferDataSource.
func (mds *mockDataSource) ReadAt(b []byte, offset int64) (int, error) {
	mds.mu.Lock()
	defer mds.mu.Unlock()

	// panic if these error during testing - the error being returned may not
	// make it all the way back to the test program because of failovers and
	// such, but this is an incorrect call that should never be made by the
	// stream buffer.
	if offset < 0 {
		panic("bad call to mocked ReadAt")
	}
	if uint64(offset+int64(len(b))) > uint64(len(mds.data)) {
		str := fmt.Sprintf("call to ReadAt is asking for data that exceeds the data size: %v - %v", offset+int64(len(b)), uint64(len(mds.data)))
		panic(str)
	}
	if uint64(offset)%mds.RequestSize() != 0 {
		panic("bad call to mocked ReadAt")
	}
	if uint64(len(b)) > uint64(len(mds.data)) {
		panic("bad call to mocked ReadAt")
	}
	if uint64(len(b)) != mds.RequestSize() && uint64(offset+int64(len(b))) != uint64(len(mds.data)) {
		panic("bad call to mocked ReadAt")
	}
	n := copy(b, mds.data[offset:])
	return n, nil
}

// SilentClose implements streamBufferDataSource.
func (mds *mockDataSource) SilentClose() {
	mds.mu.Lock()
	mds.data = nil
	mds.mu.Unlock()
}

// TestStreamSmoke checks basic logic on the stream to see that reading and
// seeking and closing works.
func TestStreamSmoke(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Create a usable stream, starting at offset 0.
	var tg threadgroup.ThreadGroup
	data := fastrand.Bytes(15999) // 1 byte short of 1000 data sections.
	dataSectionSize := uint64(16)
	dataSource := newMockDataSource(data, dataSectionSize)
	sbs := newStreamBufferSet(&tg)
	stream := sbs.callNewStream(dataSource, 0)

	// Perform the ritual that the http.ResponseWriter performs - seek to front,
	// seek to back, read 512 bytes, seek to front, read a bigger chunk of data.
	offset, err := stream.Seek(0, io.SeekStart)
	if err != nil {
		t.Fatal(err)
	}
	if offset != 0 {
		t.Fatal("bad")
	}
	offset, err = stream.Seek(0, io.SeekEnd)
	if err != nil {
		t.Fatal(err)
	}
	if offset != 15999 {
		t.Fatal("bad")
	}
	offset, err = stream.Seek(0, io.SeekStart)
	if err != nil {
		t.Fatal(err)
	}
	if offset != 0 {
		t.Fatal("bad")
	}
	buf := make([]byte, 512)
	bytesRead, err := io.ReadFull(stream, buf)
	if err != nil {
		t.Fatal(err)
	}
	if bytesRead != 512 {
		t.Fatal("bad", bytesRead)
	}
	if !bytes.Equal(buf, data[:512]) {
		t.Fatal("bad")
	}
	offset, err = stream.Seek(0, io.SeekStart)
	if err != nil {
		t.Fatal(err)
	}
	if offset != 0 {
		t.Fatal("bad")
	}
	buf = make([]byte, 1000)
	bytesRead, err = io.ReadFull(stream, buf)
	if err != nil {
		t.Fatal(err)
	}
	if bytesRead != 1000 {
		t.Fatal("bad")
	}
	if !bytes.Equal(buf, data[:1000]) {
		t.Fatal("bad")
	}
	// Seek back to the beginning one more time to do a full read of the data.
	offset, err = stream.Seek(0, io.SeekStart)
	if err != nil {
		t.Fatal(err)
	}
	if offset != 0 {
		t.Fatal("bad")
	}
	// Read all of the data.
	fullRead, err := ioutil.ReadAll(stream)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fullRead, data) {
		t.Fatal("bad")
	}

	// The reads should have exceeded the cache size. Check that the number of
	// nodes in the stream buffer and lru match what is expected.
	expectedNodes := int(bytesBufferedPerStream / dataSectionSize)
	if len(stream.staticStreamBuffer.dataSections) != expectedNodes {
		t.Fatal("bad")
	}
	if len(stream.lru.nodes) != expectedNodes {
		t.Fatal("bad")
	}

	// Open up a second stream and read the front of the data. This should cause
	// the stream buffer to have a full cache for each stream and stream2, since
	// there is no overlap between their lrus.
	//
	// NOTE: Need to use a second data source, because it'll be closed when it's
	// not used. The stream buffer expects that when multiple data sources have
	// the same ID, they are actually separate objects which need to be closed
	// individually.
	dataSource2 := newMockDataSource(data, dataSectionSize)
	stream2 := sbs.callNewStream(dataSource2, 0)
	bytesRead, err = io.ReadFull(stream2, buf)
	if err != nil {
		t.Fatal(err)
	}
	if bytesRead != len(buf) {
		t.Fatal("bad")
	}
	if !bytes.Equal(buf, data[:len(buf)]) {
		t.Fatal("bad")
	}
	if len(stream.staticStreamBuffer.dataSections) != (expectedNodes * 2) {
		t.Fatal("bad")
	}
	if len(stream2.staticStreamBuffer.dataSections) != (expectedNodes * 2) {
		t.Fatal("bad")
	}
	if len(stream.lru.nodes) != expectedNodes {
		t.Fatal("bad")
	}
	if len(stream2.lru.nodes) != expectedNodes {
		t.Fatal("bad")
	}

	// Read the full data on stream2, this should cause the lrus to match, and
	// therefore the stream buffer to only have 1 set of data.
	fullRead, err = ioutil.ReadAll(stream2)
	if err != nil {
		t.Fatal(err)
	}
	if len(stream.staticStreamBuffer.dataSections) != expectedNodes {
		t.Fatal("bad")
	}
	if len(stream2.staticStreamBuffer.dataSections) != expectedNodes {
		t.Fatal("bad")
	}
	if len(stream.lru.nodes) != expectedNodes {
		t.Fatal("bad")
	}
	if len(stream2.lru.nodes) != expectedNodes {
		t.Fatal("bad")
	}

	// Close the stream and see that all resources for the stream are dropped,
	// but that the stream buffer sticks around.
	err = stream.Close()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(keepOldBuffersDuration / 3)
	// Check that the stream hangs around a bit after close.
	stream.lru.mu.Lock()
	if len(stream.lru.nodes) == 0 {
		stream.lru.mu.Unlock()
		t.Fatal("bad")
	}
	if stream.lru.head == nil {
		stream.lru.mu.Unlock()
		t.Fatal("bad")
	}
	if stream.lru.tail == nil {
		stream.lru.mu.Unlock()
		t.Fatal("bad")
	}
	stream.lru.mu.Unlock()
	// Sleep until the stream is cleared.
	time.Sleep(keepOldBuffersDuration)
	if dataSource.data == nil {
		t.Fatal("bad")
	}
	stream.lru.mu.Lock()
	if len(stream.lru.nodes) != 0 {
		stream.lru.mu.Unlock()
		t.Fatal("bad")
	}
	if stream.lru.head != nil {
		stream.lru.mu.Unlock()
		t.Fatal("bad")
	}
	if stream.lru.tail != nil {
		stream.lru.mu.Unlock()
		t.Fatal("bad")
	}
	stream.lru.mu.Unlock()
	if len(sbs.streams) != 1 {
		t.Fatal("bad")
	}
	if len(stream.staticStreamBuffer.dataSections) != expectedNodes {
		t.Fatal("bad")
	}

	// Close the second stream and see that all resources are dropped.
	err = stream2.Close()
	if err != nil {
		t.Fatal(err)
	}
	// Sleep until the stream is cleared.
	time.Sleep(keepOldBuffersDuration / 3)
	time.Sleep(keepOldBuffersDuration)
	dataSource.mu.Lock()
	if dataSource.data != nil {
		dataSource.mu.Unlock()
		t.Fatal("bad")
	}
	dataSource.mu.Unlock()
	stream2.lru.mu.Lock()
	if len(stream2.lru.nodes) != 0 {
		stream2.lru.mu.Unlock()
		t.Fatal("bad")
	}
	if stream2.lru.head != nil {
		stream2.lru.mu.Unlock()
		t.Fatal("bad")
	}
	if stream2.lru.tail != nil {
		stream2.lru.mu.Unlock()
		t.Fatal("bad")
	}
	stream2.lru.mu.Unlock()
	if len(sbs.streams) != 0 {
		t.Fatal("bad")
	}
	if len(stream2.staticStreamBuffer.dataSections) != 0 {
		t.Fatal("bad")
	}

	// Check that if the tg is stopped, the stream closes immediately.
	dataSource3 := newMockDataSource(data, dataSectionSize)
	stream3 := sbs.callNewStream(dataSource3, 0)
	bytesRead, err = io.ReadFull(stream3, buf)
	if err != nil {
		t.Fatal(err)
	}
	err = stream3.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = tg.Stop()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(keepOldBuffersDuration / 5)
	dataSource.mu.Lock()
	if dataSource.data != nil {
		dataSource.mu.Unlock()
		t.Fatal("bad")
	}
	dataSource.mu.Unlock()
	stream3.lru.mu.Lock()
	if len(stream3.lru.nodes) != 0 {
		stream3.lru.mu.Unlock()
		t.Fatal("bad")
	}
	if stream3.lru.head != nil {
		stream3.lru.mu.Unlock()
		t.Fatal("bad")
	}
	if stream3.lru.tail != nil {
		stream3.lru.mu.Unlock()
		t.Fatal("bad")
	}
	stream3.lru.mu.Unlock()
	sbs.mu.Lock()
	if len(sbs.streams) != 0 {
		sbs.mu.Unlock()
		t.Fatal("bad")
	}
	sbs.mu.Unlock()
	if len(stream3.staticStreamBuffer.dataSections) != 0 {
		t.Fatal("bad")
	}
}
