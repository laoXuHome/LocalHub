package transfer

import (
	"fmt"

	"github.com/laoXuHome/LocalHub/internal/bookmark"
	"github.com/laoXuHome/LocalHub/internal/icon"
)

// Analyzed 是单条记录的校验与重复分析结果。
type Analyzed struct {
	Record     Record
	Bookmark   *bookmark.Bookmark
	Err        error
	IconErr    error
	Status     string // new / duplicate / invalid
	Reason     string
	ExistingID int64
	DupKind    string
}

// AnalyzeRecords 校验每条记录并做重复检测，不写数据库。
func AnalyzeRecords(recs []Record, existing []bookmark.Bookmark) ([]Analyzed, Preview) {
	existingIdx := map[string]int64{}
	for _, b := range existing {
		for _, key := range URLKeys(b.LANURL, b.WANURL) {
			if _, ok := existingIdx[key]; !ok {
				existingIdx[key] = b.ID
			}
		}
	}
	batchIdx := map[string]int{}
	out := make([]Analyzed, 0, len(recs))
	p := Preview{Total: len(recs), Items: make([]Item, 0, len(recs))}
	iconWarn := 0
	for _, rec := range recs {
		a := Analyzed{Record: rec}
		in := rec.ToInput()
		if len(rec.IconData) > 0 {
			// 内嵌图标先单独校验，书签字段按无图标校验，避免 icon_type=file 被拒。
			in.IconType = nil
			in.IconValue = nil
			_, _, err := icon.Validate(rec.IconData)
			a.IconErr = err
			if err != nil {
				iconWarn++
			}
		}
		b, err := bookmark.Build(nil, in)
		if err != nil {
			a.Err = err
			a.Status = "invalid"
			a.Reason = err.Error()
			p.Invalid++
			p.Items = append(p.Items, Item{Index: rec.Index, Name: rec.Name, LANURL: rec.LANURL, WANURL: rec.WANURL, Folder: rec.Folder, Status: "invalid", Reason: a.Reason})
			out = append(out, a)
			continue
		}
		a.Bookmark = b
		keys := URLKeys(b.LANURL, b.WANURL)
		dupID := int64(0)
		dupKind := ""
		for _, k := range keys {
			if id, ok := existingIdx[k]; ok {
				dupID, dupKind = id, "existing"
				break
			}
			if i, ok := batchIdx[k]; ok {
				dupID, dupKind = 0, fmt.Sprintf("batch:%d", i)
				break
			}
		}
		if dupKind != "" {
			a.Status = "duplicate"
			a.ExistingID = dupID
			a.DupKind = "url"
			if dupID == 0 {
				a.DupKind = "batch"
				a.Reason = "与本次导入中的第 " + dupKind[6:] + " 条重复"
			} else {
				a.Reason = fmt.Sprintf("与已有书签 ID=%d 重复", dupID)
			}
			p.Duplicates++
		} else {
			a.Status = "new"
			p.New++
		}
		for _, k := range keys {
			if _, ok := batchIdx[k]; !ok {
				batchIdx[k] = rec.Index
			}
		}
		p.Items = append(p.Items, Item{
			Index: rec.Index, Name: b.Name, LANURL: b.LANURL, WANURL: b.WANURL, Folder: rec.Folder,
			Status: a.Status, Reason: a.Reason, ExistingID: a.ExistingID, Duplicate: a.DupKind,
		})
		out = append(out, a)
	}
	p.Valid = p.New + p.Duplicates
	if iconWarn > 0 {
		p.Warnings = append(p.Warnings, Warning{Code: "icon_invalid", Message: fmt.Sprintf("%d 条记录的内嵌图标未通过安全校验，导入时将只保留书签、不保存图标", iconWarn)})
	}
	return out, p
}
