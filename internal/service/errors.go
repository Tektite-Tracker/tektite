package service

import (
	"errors"
	"fmt"

	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/internal/store"
)

var kindNames = map[Kind]string{
	KindInternal: "internal",
	KindUsage:    "usage",
	KindNotFound: "not_found",
	KindConflict: "conflict",
	KindPolicy:   "policy",
}

func (k Kind) String() string {
	return kindNames[k]
}

func Classify(err error) *Error {
	var se *Error
	var amb *node.AmbiguousError
	switch {
	case errors.As(err, &se):
		return se
	case errors.As(err, &amb):
		return &Error{Kind: KindUsage, Msg: err.Error(), Hint: "use a longer hash to pick one"}
	case errors.Is(err, node.ErrNotFound):
		return &Error{Kind: KindNotFound, Msg: err.Error(), Hint: "run `tektite ls` to list references"}
	case errors.Is(err, node.ErrTooShort):
		return &Error{Kind: KindUsage, Msg: err.Error(), Hint: fmt.Sprintf("use at least %d hash characters", node.MinHashLen)}
	case errors.Is(err, store.ErrNoRepo):
		return &Error{Kind: KindNotFound, Msg: err.Error(), Hint: "run `tektite init` to create one"}
	case errors.Is(err, store.ErrExists):
		return &Error{Kind: KindConflict, Msg: err.Error(), Hint: "retry the command"}
	}
	return &Error{Kind: KindInternal, Msg: err.Error()}
}
