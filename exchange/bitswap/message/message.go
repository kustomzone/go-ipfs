package message

import (
	"fmt"
	"io"

	blocks "github.com/ipfs/go-ipfs/blocks"
	pb "github.com/ipfs/go-ipfs/exchange/bitswap/message/pb"
	wantlist "github.com/ipfs/go-ipfs/exchange/bitswap/wantlist"

	ggio "gx/ipfs/QmZ4Qi3GaRbjcx28Sme5eMH7RQjGkt8wHxt2a65oLaeFEV/gogo-protobuf/io"
	proto "gx/ipfs/QmZ4Qi3GaRbjcx28Sme5eMH7RQjGkt8wHxt2a65oLaeFEV/gogo-protobuf/proto"
	cid "gx/ipfs/QmakyCk6Vnn16WEKjbkxieZmM2YLTzkFWizbmGowoYPjro/go-cid"
	inet "gx/ipfs/QmdXimY9QHaasZmw6hWojWnCJvfgxETjZQfg9g6ZrA9wMX/go-libp2p-net"
)

// TODO move message.go into the bitswap package
// TODO move bs/msg/internal/pb to bs/internal/pb and rename pb package to bitswap_pb

type BitSwapMessage interface {
	// Wantlist returns a slice of unique keys that represent data wanted by
	// the sender.
	Wantlist() []Entry

	// Blocks returns a slice of unique blocks
	Blocks() []blocks.Block

	// AddEntry adds an entry to the Wantlist.
	AddEntry(key *cid.Cid, priority int)

	Cancel(key *cid.Cid)

	Empty() bool

	// A full wantlist is an authoritative copy, a 'non-full' wantlist is a patch-set
	Full() bool

	AddBlock(blocks.Block)
	Exportable

	Loggable() map[string]interface{}
}

type Exportable interface {
	ToProto() *pb.Message
	ToNet(w io.Writer) error
}

type impl struct {
	full     bool
	wantlist map[string]Entry
	blocks   map[string]blocks.Block
}

func New(full bool) BitSwapMessage {
	return newMsg(full)
}

func newMsg(full bool) *impl {
	return &impl{
		blocks:   make(map[string]blocks.Block),
		wantlist: make(map[string]Entry),
		full:     full,
	}
}

type Entry struct {
	*wantlist.Entry
	Cancel bool
}

func newMessageFromProto(pbm pb.Message) (BitSwapMessage, error) {
	m := newMsg(pbm.GetWantlist().GetFull())
	for _, e := range pbm.GetWantlist().GetEntries() {
		c, err := cid.Cast([]byte(e.GetBlock()))
		if err != nil {
			return nil, fmt.Errorf("incorrectly formatted cid in wantlist: %s", err)
		}
		m.addEntry(c, int(e.GetPriority()), e.GetCancel())
	}
	for _, d := range pbm.GetBlocks() {
		b := blocks.NewBlock(d)
		m.AddBlock(b)
	}
	return m, nil
}

func (m *impl) Full() bool {
	return m.full
}

func (m *impl) Empty() bool {
	return len(m.blocks) == 0 && len(m.wantlist) == 0
}

func (m *impl) Wantlist() []Entry {
	var out []Entry
	for _, e := range m.wantlist {
		out = append(out, e)
	}
	return out
}

func (m *impl) Blocks() []blocks.Block {
	bs := make([]blocks.Block, 0, len(m.blocks))
	for _, block := range m.blocks {
		bs = append(bs, block)
	}
	return bs
}

func (m *impl) Cancel(k *cid.Cid) {
	delete(m.wantlist, k.KeyString())
	m.addEntry(k, 0, true)
}

func (m *impl) AddEntry(k *cid.Cid, priority int) {
	m.addEntry(k, priority, false)
}

func (m *impl) addEntry(c *cid.Cid, priority int, cancel bool) {
	k := c.KeyString()
	e, exists := m.wantlist[k]
	if exists {
		e.Priority = priority
		e.Cancel = cancel
	} else {
		m.wantlist[k] = Entry{
			Entry: &wantlist.Entry{
				Cid:      c,
				Priority: priority,
			},
			Cancel: cancel,
		}
	}
}

func (m *impl) AddBlock(b blocks.Block) {
	m.blocks[b.Cid().KeyString()] = b
}

func FromNet(r io.Reader) (BitSwapMessage, error) {
	pbr := ggio.NewDelimitedReader(r, inet.MessageSizeMax)
	return FromPBReader(pbr)
}

func FromPBReader(pbr ggio.Reader) (BitSwapMessage, error) {
	pb := new(pb.Message)
	if err := pbr.ReadMsg(pb); err != nil {
		return nil, err
	}

	return newMessageFromProto(*pb)
}

func (m *impl) ToProto() *pb.Message {
	pbm := new(pb.Message)
	pbm.Wantlist = new(pb.Message_Wantlist)
	for _, e := range m.wantlist {
		pbm.Wantlist.Entries = append(pbm.Wantlist.Entries, &pb.Message_Wantlist_Entry{
			Block:    proto.String(e.Cid.KeyString()),
			Priority: proto.Int32(int32(e.Priority)),
			Cancel:   proto.Bool(e.Cancel),
		})
	}
	for _, b := range m.Blocks() {
		pbm.Blocks = append(pbm.Blocks, b.RawData())
	}
	return pbm
}

func (m *impl) ToNet(w io.Writer) error {
	pbw := ggio.NewDelimitedWriter(w)

	if err := pbw.WriteMsg(m.ToProto()); err != nil {
		return err
	}
	return nil
}

func (m *impl) Loggable() map[string]interface{} {
	var blocks []string
	for _, v := range m.blocks {
		blocks = append(blocks, v.Cid().String())
	}
	return map[string]interface{}{
		"blocks": blocks,
		"wants":  m.Wantlist(),
	}
}
