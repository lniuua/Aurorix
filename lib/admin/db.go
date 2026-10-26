package admin

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/storage"
)

// 数据库管理：表结构浏览 + 行级增删改查 + 单条 SQL 执行。
//
// 安全边界：表名/列名一律先用 sqlite_master 与 pragma_table_info 校验，通过后才用
// quoteIdent 拼进 SQL —— 用户输入的标识符永远不直接当 SQL 片段用。
// 行数据本身全部走占位符。

const (
	dbDefaultLimit = 50
	dbMaxLimit     = 200
	dbQueryRowCap  = 500
)

type dbColumnInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	NotNull bool   `json:"not_null"`
	Default any    `json:"default"`
	PK      int    `json:"pk"`
}

// handleDBTables 列出所有用户表及其行数。
func handleDBTables(w http.ResponseWriter, r *http.Request) {
	db, ok := dbHandle(w)
	if !ok {
		return
	}
	names, err := dbTableNames(db)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}
	out := make([]H, 0, len(names))
	for _, name := range names {
		out = append(out, H{"name": name, "rows": dbCount(db, `SELECT count(*) FROM `+quoteIdent(name), nil)})
	}
	writeJSON(w, http.StatusOK, H{"tables": out})
}

// handleDBTable 返回单表结构（列信息 + 主键 + 行数）。
func handleDBTable(w http.ResponseWriter, r *http.Request) {
	db, ok := dbHandle(w)
	if !ok {
		return
	}
	table, ok := dbTableParam(w, r, db)
	if !ok {
		return
	}
	cols, err := dbTableColumns(db, table)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}
	pk := dbPrimaryKey(cols)
	writeJSON(w, http.StatusOK, H{
		"name":     table,
		"columns":  cols,
		"pk":       pk,
		"editable": len(pk) > 0,
		"rows":     dbCount(db, `SELECT count(*) FROM `+quoteIdent(table), nil),
	})
}

// handleDBRows 分页读取行数据；q 会在所有文本列上做 LIKE。
func handleDBRows(w http.ResponseWriter, r *http.Request) {
	db, ok := dbHandle(w)
	if !ok {
		return
	}
	table, ok := dbTableParam(w, r, db)
	if !ok {
		return
	}
	cols, err := dbTableColumns(db, table)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}

	q := r.URL.Query()
	limit := dbDefaultLimit
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 {
		limit = min(v, dbMaxLimit)
	}
	offset := 0
	if v, err := strconv.Atoi(q.Get("offset")); err == nil && v > 0 {
		offset = v
	}

	where, args := dbSearchWhere(cols, q.Get("q"))
	order := ""
	if name := q.Get("order"); name != "" && dbHasColumn(cols, name) {
		dir := "ASC"
		if strings.EqualFold(q.Get("dir"), "desc") {
			dir = "DESC"
		}
		order = " ORDER BY " + quoteIdent(name) + " " + dir
	}

	query := `SELECT * FROM ` + quoteIdent(table) + where + order +
		` LIMIT ? OFFSET ?`
	queryArgs := append(append([]any{}, args...), limit, offset)
	rows, err := db.Query(query, queryArgs...)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}
	defer rows.Close()

	names, data, err := dbScanRows(rows, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, H{
		"table":  table,
		"columns": names,
		"rows":   data,
		"total":  dbCount(db, `SELECT count(*) FROM `+quoteIdent(table)+where, args),
		"limit":  limit,
		"offset": offset,
	})
}

// handleDBInsertRow 插入一行；values 里出现未知列会被忽略。
func handleDBInsertRow(w http.ResponseWriter, r *http.Request) {
	db, ok := dbHandle(w)
	if !ok {
		return
	}
	table, ok := dbTableParam(w, r, db)
	if !ok {
		return
	}
	cols, err := dbTableColumns(db, table)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}
	var in dbRowInput
	if err := readJSON(r, &in); err != nil {
		writeJSON(w, http.StatusBadRequest, H{"error": "请求格式无效"})
		return
	}

	names, args := dbSplitValues(cols, in.Values)
	var sqlText string
	if len(names) == 0 {
		sqlText = `INSERT INTO ` + quoteIdent(table) + ` DEFAULT VALUES`
	} else {
		marks := make([]string, len(names))
		quoted := make([]string, len(names))
		for i, name := range names {
			marks[i] = "?"
			quoted[i] = quoteIdent(name)
		}
		sqlText = `INSERT INTO ` + quoteIdent(table) + ` (` + strings.Join(quoted, ", ") +
			`) VALUES (` + strings.Join(marks, ", ") + `)`
	}
	res, err := db.Exec(sqlText, args...)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, http.StatusOK, H{"ok": true, "last_insert_id": id})
}

// handleDBUpdateRow 按主键更新一行。
func handleDBUpdateRow(w http.ResponseWriter, r *http.Request) {
	db, ok := dbHandle(w)
	if !ok {
		return
	}
	table, ok := dbTableParam(w, r, db)
	if !ok {
		return
	}
	cols, err := dbTableColumns(db, table)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}
	pk := dbPrimaryKey(cols)
	if len(pk) == 0 {
		writeJSON(w, http.StatusBadRequest, H{"error": "该表没有主键，无法定位要更新的行"})
		return
	}
	var in dbRowInput
	if err := readJSON(r, &in); err != nil {
		writeJSON(w, http.StatusBadRequest, H{"error": "请求格式无效"})
		return
	}

	names, args := dbSplitValues(cols, in.Values)
	if len(names) == 0 {
		writeJSON(w, http.StatusBadRequest, H{"error": "没有要更新的字段"})
		return
	}
	sets := make([]string, len(names))
	for i, name := range names {
		sets[i] = quoteIdent(name) + " = ?"
	}
	where, whereArgs, err := dbKeyWhere(pk, in.Key)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}
	res, err := db.Exec(`UPDATE `+quoteIdent(table)+` SET `+strings.Join(sets, ", ")+` WHERE `+where,
		append(args, whereArgs...)...)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}
	affected, _ := res.RowsAffected()
	writeJSON(w, http.StatusOK, H{"ok": true, "affected": affected})
}

// handleDBDeleteRow 按主键删除一行。
func handleDBDeleteRow(w http.ResponseWriter, r *http.Request) {
	db, ok := dbHandle(w)
	if !ok {
		return
	}
	table, ok := dbTableParam(w, r, db)
	if !ok {
		return
	}
	cols, err := dbTableColumns(db, table)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return
	}
	pk := dbPrimaryKey(cols)
	if len(pk) == 0 {
		writeJSON(w, http.StatusBadRequest, H{"error": "该表没有主键，无法定位要删除的行"})
		return
	}
	var in dbRowInput
	if err := readJSON(r, &in); err != nil {
		writeJSON(w, http.StatusBadRequest, H{"error": "请求格式无效"})
		return
	}
	where, args, err := dbKeyWhere(pk, in.Key)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}
	res, err := db.Exec(`DELETE FROM `+quoteIdent(table)+` WHERE `+where, args...)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}
	affected, _ := res.RowsAffected()
	writeJSON(w, http.StatusOK, H{"ok": true, "affected": affected})
}

// handleDBQuery 执行单条 SQL：查询类返回结果集，其余返回影响行数。
func handleDBQuery(w http.ResponseWriter, r *http.Request) {
	db, ok := dbHandle(w)
	if !ok {
		return
	}
	var in struct {
		SQL string `json:"sql"`
	}
	if err := readJSON(r, &in); err != nil {
		writeJSON(w, http.StatusBadRequest, H{"error": "请求格式无效"})
		return
	}
	raw := strings.TrimSpace(in.SQL)
	if raw == "" {
		writeJSON(w, http.StatusBadRequest, H{"error": "SQL 不能为空"})
		return
	}
	if sqlExtraStatements(raw) > 0 {
		writeJSON(w, http.StatusBadRequest, H{"error": "一次只能执行一条语句"})
		return
	}

	start := time.Now()
	if sqlIsReadOnly(raw) {
		rows, err := db.Query(raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, H{"error": err.Error()})
			return
		}
		defer rows.Close()
		names, data, err := dbScanRows(rows, dbQueryRowCap)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, H{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, H{
			"kind":       "rows",
			"columns":    names,
			"rows":       data,
			"elapsed_ms": time.Since(start).Milliseconds(),
		})
		return
	}

	res, err := db.Exec(raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}
	affected, _ := res.RowsAffected()
	writeJSON(w, http.StatusOK, H{
		"kind":          "exec",
		"affected":      affected,
		"elapsed_ms":    time.Since(start).Milliseconds(),
	})
}

type dbRowInput struct {
	Values map[string]any `json:"values"`
	Key    map[string]any `json:"key"`
}

func dbHandle(w http.ResponseWriter) (*sql.DB, bool) {
	db, err := storage.DB()
	if err == nil && db != nil {
		return db, true
	}
	msg := "数据库未打开"
	if err != nil {
		msg = err.Error()
	}
	writeJSON(w, http.StatusServiceUnavailable, H{"error": msg})
	return nil, false
}

func dbTableParam(w http.ResponseWriter, r *http.Request, db *sql.DB) (string, bool) {
	name := r.PathValue("table")
	names, err := dbTableNames(db)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, H{"error": err.Error()})
		return "", false
	}
	for _, candidate := range names {
		if candidate == name {
			return candidate, true
		}
	}
	writeJSON(w, http.StatusNotFound, H{"error": "表不存在"})
	return "", false
}

func dbTableNames(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT name FROM sqlite_master
		WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

func dbTableColumns(db *sql.DB, table string) ([]dbColumnInfo, error) {
	rows, err := db.Query(`SELECT name, type, "notnull", dflt_value, pk FROM pragma_table_info(?)`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []dbColumnInfo
	for rows.Next() {
		var (
			name, typ string
			notNull   int
			dflt      sql.NullString
			pk        int
		)
		if err := rows.Scan(&name, &typ, &notNull, &dflt, &pk); err != nil {
			return nil, err
		}
		col := dbColumnInfo{Name: name, Type: typ, NotNull: notNull != 0, PK: pk}
		if dflt.Valid {
			col.Default = dflt.String
		}
		out = append(out, col)
	}
	return out, rows.Err()
}

func dbPrimaryKey(cols []dbColumnInfo) []string {
	var pk []string
	for _, col := range cols {
		if col.PK > 0 {
			pk = append(pk, col.Name)
		}
	}
	return pk
}

func dbHasColumn(cols []dbColumnInfo, name string) bool {
	for _, col := range cols {
		if col.Name == name {
			return true
		}
	}
	return false
}

// dbSearchWhere 用 q 在文本列上做 LIKE；没有文本列时退化为不过滤。
func dbSearchWhere(cols []dbColumnInfo, q string) (string, []any) {
	q = strings.TrimSpace(q)
	if q == "" {
		return "", nil
	}
	var parts []string
	var args []any
	for _, col := range cols {
		if !dbIsTextType(col.Type) {
			continue
		}
		parts = append(parts, `CAST(`+quoteIdent(col.Name)+` AS TEXT) LIKE ?`)
		args = append(args, "%"+q+"%")
	}
	if len(parts) == 0 {
		return "", nil
	}
	return " WHERE (" + strings.Join(parts, " OR ") + ")", args
}

func dbIsTextType(typ string) bool {
	up := strings.ToUpper(typ)
	return strings.Contains(up, "CHAR") || strings.Contains(up, "TEXT") || strings.Contains(up, "CLOB") || up == ""
}

// dbSplitValues 只保留真实存在的列，返回列名与对应参数。
func dbSplitValues(cols []dbColumnInfo, values map[string]any) ([]string, []any) {
	names := make([]string, 0, len(values))
	args := make([]any, 0, len(values))
	for _, col := range cols {
		v, ok := values[col.Name]
		if !ok {
			continue
		}
		names = append(names, col.Name)
		args = append(args, dbNormalizeValue(v))
	}
	return names, args
}

// dbNormalizeValue 把前端传来的数字/字符串收敛成 SQLite 能存的类型。
func dbNormalizeValue(v any) any {
	switch t := v.(type) {
	case float64:
		// JSON 数字一律是 float64；整数就还原成 int64，别把 1 写成 1.0
		if t == float64(int64(t)) {
			return int64(t)
		}
		return t
	case map[string]any, []any:
		return fmt.Sprintf("%v", t)
	default:
		return v
	}
}

func dbKeyWhere(pk []string, key map[string]any) (string, []any, error) {
	if len(pk) == 0 {
		return "", nil, fmt.Errorf("该表没有主键，无法定位行")
	}
	parts := make([]string, 0, len(pk))
	args := make([]any, 0, len(pk))
	for _, col := range pk {
		v, ok := key[col]
		if !ok {
			return "", nil, fmt.Errorf("缺少主键 %s", col)
		}
		// IS 与 = 的区别只在 NULL：主键理论上非空，但用 IS 能兼容老数据
		parts = append(parts, quoteIdent(col)+" IS ?")
		args = append(args, dbNormalizeValue(v))
	}
	return strings.Join(parts, " AND "), args, nil
}

func dbCount(db *sql.DB, query string, args []any) int64 {
	var n int64
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		return -1
	}
	return n
}

func dbScanRows(rows *sql.Rows, maxRows int) ([]string, [][]any, error) {
	names, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	out := make([][]any, 0, 32)
	for rows.Next() {
		if maxRows > 0 && len(out) >= maxRows {
			break
		}
		vals := make([]any, len(names))
		ptrs := make([]any, len(names))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, err
		}
		row := make([]any, len(names))
		for i, v := range vals {
			row[i] = dbEncodeValue(v)
		}
		out = append(out, row)
	}
	return names, out, rows.Err()
}

func dbEncodeValue(v any) any {
	switch t := v.(type) {
	case []byte:
		return string(t)
	case time.Time:
		return t.Format(time.RFC3339)
	default:
		return v
	}
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func sqlIsReadOnly(sqlText string) bool {
	word := strings.ToLower(sqlFirstWord(sqlText))
	switch word {
	case "select", "pragma", "explain", "values", "with":
		return true
	default:
		return false
	}
}

func sqlFirstWord(sqlText string) string {
	sqlText = strings.TrimLeft(sqlText, " \t\r\n(")
	end := strings.IndexAny(sqlText, " \t\r\n(")
	if end < 0 {
		return sqlText
	}
	return sqlText[:end]
}

// sqlExtraStatements 数「分号后面还有内容」的次数，忽略引号与行注释里的分号。
func sqlExtraStatements(sqlText string) int {
	extra := 0
	inSingle, inDouble, inBacktick := false, false, false
	for i := 0; i < len(sqlText); i++ {
		c := sqlText[i]
		switch {
		case inSingle:
			if c == '\'' {
				if i+1 < len(sqlText) && sqlText[i+1] == '\'' {
					i++
				} else {
					inSingle = false
				}
			}
		case inDouble:
			if c == '"' {
				if i+1 < len(sqlText) && sqlText[i+1] == '"' {
					i++
				} else {
					inDouble = false
				}
			}
		case inBacktick:
			if c == '`' {
				inBacktick = false
			}
		case c == '\'':
			inSingle = true
		case c == '"':
			inDouble = true
		case c == '`':
			inBacktick = true
		case c == '-' && i+1 < len(sqlText) && sqlText[i+1] == '-':
			for i < len(sqlText) && sqlText[i] != '\n' {
				i++
			}
		case c == ';':
			if strings.TrimSpace(sqlText[i+1:]) != "" {
				extra++
			}
		}
	}
	return extra
}
