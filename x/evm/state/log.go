package state

import (
	"encoding/json"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/sei-protocol/sei-chain/x/evm/types"
)

type Logs struct {
	Ls []*ethtypes.Log `json:"logs"`
}

func (s *StateDBImpl) AddLog(l *ethtypes.Log) {
	// TODO: potentially decorate log with block/tx metadata
	store := s.k.PrefixStore(s.ctx, types.TransientModuleStateKeyPrefix)
	logs := Logs{Ls: []*ethtypes.Log{}}
	if ls, err := s.GetLogs(); err != nil {
		s.err = err
		return
	} else {
		logs.Ls = append(ls, l)
	}
	logsbz, err := json.Marshal(&logs)
	if err != nil {
		s.err = err
		return
	}
	store.Set(LogsKey, logsbz)
}

func (s *StateDBImpl) GetLogs() ([]*ethtypes.Log, error) {
	store := s.k.PrefixStore(s.ctx, types.TransientModuleStateKeyPrefix)
	logsbz := store.Get(LogsKey)
	logs := Logs{Ls: []*ethtypes.Log{}}
	if logsbz == nil {
		return []*ethtypes.Log{}, nil
	}
	if err := json.Unmarshal(logsbz, &logs); err != nil {
		return []*ethtypes.Log{}, err
	}
	return logs.Ls, nil
}
