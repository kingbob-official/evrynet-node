package tendermint

import (
	"io"
	"strconv"

	"github.com/evrynet-official/evrynet-client/core/types"
	"github.com/evrynet-official/evrynet-client/rlp"
)

//Proposal represent a propose message to be sent in the case of the node is a proposer
//for its Round.
type Proposal struct {
	Block    *types.Block
	Round    int64
	POLRound int64
	//TODO: check if we need block Height
}

func (p *Proposal) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{
		p.Block,
		strconv.FormatInt(p.Round, 10),
		strconv.FormatInt(p.POLRound, 10),
	})
}

func (p *Proposal) DecodeRLP(s *rlp.Stream) error {
	var ps struct {
		Block   *types.Block
		RStr    string
		POLRStr string
	}
	if err := s.Decode(&ps); err != nil {
		return err
	}
	round, err := strconv.ParseInt(ps.RStr, 10, 64)
	if err != nil {
		return err
	}
	polcr, err := strconv.ParseInt(ps.POLRStr, 10, 64)
	if err != nil {
		return err
	}
	p.Block = ps.Block
	p.Round = round
	p.POLRound = polcr
	return nil
}
