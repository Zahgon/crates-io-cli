// Package list is a port of src/scmds/list/: the `crates list` subcommand.
package list

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"

	"github.com/Byron/crates-io-cli/internal/httputil"
	"github.com/Byron/crates-io-cli/internal/prettytable"
	"github.com/Byron/crates-io-cli/internal/structs"
)

// Error mirrors the list subcommand's error enum. Its messages are printed by
// ok_or_exit and so are part of the observable surface.
type Error struct {
	Kind ErrorKind
	Err  error
}

// ErrorKind identifies which variant an Error holds.
type ErrorKind uint8

// The variants of the list `Error` enum.
const (
	// ErrDecodeJSON: "Json from the server could not be decoded"
	ErrDecodeJSON ErrorKind = iota
	// ErrEasy: "A remote call could not be performed"
	ErrEasy
	// ErrIO: "Output could not be written"
	ErrIO
)

func (e *Error) Error() string {
	switch e.Kind {
	case ErrDecodeJSON:
		return "Json from the server could not be decoded"
	case ErrEasy:
		return "A remote call could not be performed"
	default:
		return "Output could not be written"
	}
}

func (e *Error) Unwrap() error { return e.Err }

// CratesFromCallResultBuf is a port of `crates_from_callresult_buf`.
func CratesFromCallResultBuf(buf []byte) ([]structs.Crate, structs.Meta, error) {
	var c structs.Crates
	if err := json.Unmarshal(buf, &c); err != nil {
		return nil, structs.Meta{}, &Error{Kind: ErrDecodeJSON, Err: err}
	}
	return c.Crates, c.Meta, nil
}

func cratesMerge(r []structs.Crate, c httputil.CallResult) ([]structs.Crate, error) {
	res, _, err := CratesFromCallResultBuf(c)
	if err != nil {
		return nil, err
	}
	return append(r, res...), nil
}

func cratesExtract(c httputil.CallResult) (httputil.CallMetaData, []structs.Crate, error) {
	crates, meta, err := CratesFromCallResultBuf(c)
	if err != nil {
		return httputil.CallMetaData{}, nil, err
	}
	return httputil.CallMetaData{
		Total: meta.Total,
		Items: uint32(len(crates)),
	}, crates, nil
}

// ByUser is a port of `by_user`: every crate owned by a numeric user id.
func ByUser(ctx context.Context, client httputil.Client, id uint32) ([]structs.Crate, error) {
	target := fmt.Sprintf(
		"https://crates.io/api/v1/crates?user_id=%s",
		url.QueryEscape(strconv.FormatUint(uint64(id), 10)),
	)
	crates, err := httputil.PagedCratesIoRemoteCall(
		ctx, client, target, nil, []structs.Crate(nil), cratesMerge, cratesExtract,
	)
	if err != nil {
		var listErr *Error
		if ok := asListError(err, &listErr); ok {
			return nil, listErr
		}
		return nil, &Error{Kind: ErrEasy, Err: err}
	}
	return crates, nil
}

func asListError(err error, out **Error) bool {
	for e := err; e != nil; {
		if le, ok := e.(*Error); ok {
			*out = le
			return true
		}
		u, ok := e.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		e = u.Unwrap()
	}
	return false
}

// OutputKind mirrors the `OutputKind` value enum.
type OutputKind int

// The output formats.
const (
	OutputHuman OutputKind = iota
	OutputJSON
)

// Handle is a port of `handle_list`.
//
// The empty case returns before printing anything at all, which is why
// `crates list by-user 0` produces no output rather than an empty table.
func Handle(w io.Writer, format OutputKind, boldTitles bool, crates []structs.Crate) error {
	switch format {
	case OutputHuman:
		if len(crates) == 0 {
			return nil
		}
		t := prettytable.New()
		var total int64
		for _, c := range crates {
			total += c.Downloads
			t.AddRow([]string{
				c.Name,
				c.DescriptionOrEmpty(),
				strconv.FormatInt(c.Downloads, 10),
				c.MaxVersion,
			})
		}
		t.SetTitles([]string{
			"Name",
			"Description",
			fmt.Sprintf("Downloads (total=%d)", total),
			"MaxVersion",
		})
		if err := t.Print(w, boldTitles); err != nil {
			return &Error{Kind: ErrIO, Err: err}
		}
		return nil
	default:
		// serde serialises an empty Vec as "[]", so the JSON path prints that
		// rather than returning early like the human path.
		if crates == nil {
			crates = []structs.Crate{}
		}
		if err := structs.WriteJSONPretty(w, crates); err != nil {
			return &Error{Kind: ErrDecodeJSON, Err: err}
		}
		return nil
	}
}
