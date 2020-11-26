package libraries

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	jsoniter "github.com/json-iterator/go"
)

const (
	text_pk_type_str = "varchar(255)"
	Uintptr_offset   = 32 << (^uint(0) >> 63) / 8
)

type Mysql struct {
	db          *MysqlDB
	storeEngine string
}

//mysql结构
type Mysql_columns struct {
	Name        string
	Sql_type    string
	Null        string
	Sql_default interface{}
	Primary     bool
	Autoinc     bool
}

type Transaction struct {
	Connect *Mysql_Conn
	DB      *MysqlDB
	*Mysql_build
}

/*执行select专用
 *返回数据结构模式[]map[string]string
 */
func (this *Mysql) Query(sql string) (maps []map[string]string, err error) {
	return this.Query_Select(sql, nil)
}
func (this *Mysql) QueryString(format string, i ...interface{}) (maps []map[string]string, err error) {
	if len(i) == 0 {
		return this.Query_Select(format, nil)
	}

	str := make([]interface{}, len(i))
	for k, v := range i {
		str[k] = getvalue(v)
	}
	sql := fmt.Sprintf(strings.Replace(format, "?", `%s`, -1), str...)
	return this.Query_Select(sql, nil)
}
func (this *Mysql) Query_Select(select_sql string, t *Transaction) (maps []map[string]string, err error) {
	if this == nil || this.db == nil {
		err = errors.New("数据库未启动或者session未Begin")
		return
	}
	var rows = rows_pool.Get().(*MysqlRows)
	var columns [][]byte
	defer rows_pool.Put(rows)
Retry:
	if t != nil && t.Connect != nil {
		columns, err = t.Connect.Query([]byte(select_sql), rows)
		if err != nil {
			return
		}
	} else {
		columns, err = this.db.Query([]byte(select_sql), rows)
		if err != nil {
			if strings.Contains(err.Error(), "EOF") {
				goto Retry
			} else if strings.Contains(err.Error(), "broken pipe") { //unix断连
				goto Retry
			} else {
				return nil, errors.New(err.Error() + ",sql:" + string(select_sql))
			}
		}
	}

	if rows.result_len == 0 {
		return nil, nil
	}
	maps = make([]map[string]string, rows.result_len)
	for index, mglen := range rows.msg_len {
		rows.Buffer2.Reset()
		rows.Buffer2.Write(rows.Buffer.Next(mglen))
		//将行数据保存到record字典
		record := make(map[string]string, len(columns))
		for _, key := range columns {
			rows.buffer, err = ReadLength_Coded_Byte(rows.Buffer2)
			if err != nil {
				return
			}
			record[string(key)] = string(rows.buffer)
		}
		maps[index] = record
	}
	return
}

type sliceHeader struct {
	Data unsafe.Pointer
	Len  int
	Cap  int
}

var Field_M sync.Map //MySQL字段名称与struct字段映射
func (this *Mysql) Select(select_sql []byte, t *Transaction, r interface{}) (res bool, err error) {
	if this == nil || this.db == nil {
		err = errors.New("数据库未启动或者session未Begin")
		return
	}
	var is_struct, is_ptr bool
	var obj reflect.Value
	var type_struct, obj_t reflect.Type
	var field_m *sync.Map
	var header *sliceHeader
	var ref_ptr unsafe.Pointer
	obj = reflect.Indirect(reflect.ValueOf(r))
	switch obj.Kind() {
	case reflect.Slice:
		//取出slice里面的类型
		obj_t = obj.Type()
		type_struct = obj_t.Elem()
		if type_struct.Kind() == reflect.Struct {

		} else if type_struct.Kind() == reflect.Ptr {
			type_struct = type_struct.Elem()
			if type_struct.Kind() == reflect.Struct {
				is_ptr = true
			}
		}
		header = (*sliceHeader)(unsafe.Pointer(obj.Addr().Pointer()))
	case reflect.Struct:
		type_struct = obj.Type()
		if v, ok := Field_M.Load(type_struct.Name()); ok {
			field_m = v.(*sync.Map)
		} else {
			field_m = new(sync.Map)
			Field_M.Store(type_struct.Name(), field_m)
		}
		is_struct = true
		ref_ptr = unsafe.Pointer(obj.Addr().Pointer())
	case reflect.Ptr:
		type_struct = obj.Type()
		switch type_struct.Kind() {
		case reflect.Ptr:
			if type_struct = type_struct.Elem(); type_struct.Kind() == reflect.Struct {
				is_struct = true
				is_ptr = true
				if obj.Elem().Kind() == reflect.Invalid {
					obj.Set(reflect.New(type_struct))
				}

				ref_ptr = unsafe.Pointer(obj.Addr().Pointer())
			} else {
				err = errors.New("不支持的反射类型")
				return
			}

		case reflect.Struct:
			ref_ptr = unsafe.Pointer(obj.Addr().Pointer())
		default:
			err = errors.New("不支持的反射类型")
			return
		}
	default:
		err = errors.New("不支持的反射类型")
		return
	}
	if v, ok := Field_M.Load(type_struct.Name()); ok {
		field_m = v.(*sync.Map)
	} else {
		field_m = new(sync.Map)
		Field_M.Store(type_struct.Name(), field_m)
	}

	var rows = rows_pool.Get().(*MysqlRows)
	var columns [][]byte
	defer rows_pool.Put(rows)
Retry:

	if t != nil && t.Connect != nil {
		columns, err = t.Connect.Query(select_sql, rows)
		if err != nil {
			return
		}
	} else {

		columns, err = this.db.Query(select_sql, rows)
		if err != nil {
			if strings.Contains(err.Error(), "EOF") {
				goto Retry
			} else if strings.Contains(err.Error(), "broken pipe") { //unix断连
				goto Retry
			} else {
				return false, err
			}
		}
	}
	if rows.result_len == 0 {
		return false, nil
	}

	//var ref reflect.Value
	var field_struct *Field_struct
	var uint_ptr, offset uintptr

	if is_struct {
		offset = 0
		rows.msg_len = rows.msg_len[:1]
	} else {
		if header.Len < rows.result_len {
			if obj.Cap() < rows.result_len {
				obj.SetLen(0)
				obj.Set(reflect.AppendSlice(obj, reflect.MakeSlice(obj_t, rows.result_len, rows.result_len))) //创建一堆空nil指针或struct本体
			} else {
				obj.SetLen(rows.result_len)
			}
		}
		ref_ptr = header.Data
		if is_ptr {
			offset = Uintptr_offset
		} else {
			offset = type_struct.Size()
		}

	}

	for index, mglen := range rows.msg_len {
		uint_ptr = uintptr(ref_ptr) + offset*uintptr(index)
		if is_ptr {
			if *(*interface{})(unsafe.Pointer(uint_ptr)) == nil {
				*((*uintptr)(unsafe.Pointer(uint_ptr))) = reflect.New(type_struct).Pointer()
			}
			uint_ptr = *(*uintptr)(unsafe.Pointer(uint_ptr)) //获取指针真正的地址
		}

		rows.Buffer2.Reset()
		rows.Buffer2.Write(rows.Buffer.Next(mglen))

		for _, key := range columns {

			rows.buffer, err = ReadLength_Coded_Byte(rows.Buffer2)
			if err != nil {
				return false, err
			}

			if v, ok := field_m.Load(string(key)); ok {
				if v.(*Field_struct).Kind == reflect.Invalid {
					continue
				}
				field_struct = v.(*Field_struct)
			} else {
				real_key := string(key)
				key[0] = bytes.ToUpper(key[:1])[0]
				field, ok := type_struct.FieldByName(string(key))
				if !ok {
					DEBUG("mysql.Select()反射struct无法写入字段" + string(key) + "sql: " + string(select_sql))
					field_m.Store(real_key, &Field_struct{Kind: reflect.Invalid})
					continue
				}

				field_struct = &Field_struct{Offset: field.Offset, Kind: field.Type.Kind(), Field_t: field.Type}
				field_m.Store(real_key, field_struct)
			}

			switch field_struct.Kind {
			case reflect.Int:
				ii, _ := strconv.Atoi(*(*string)(unsafe.Pointer(&rows.buffer)))
				*((*int)(unsafe.Pointer(uint_ptr + field_struct.Offset))) = ii
			case reflect.Int8:
				ii, _ := strconv.Atoi(*(*string)(unsafe.Pointer(&rows.buffer)))
				*((*int8)(unsafe.Pointer(uint_ptr + field_struct.Offset))) = int8(ii)
			case reflect.Int16:
				ii, _ := strconv.Atoi(*(*string)(unsafe.Pointer(&rows.buffer)))
				*((*int16)(unsafe.Pointer(uint_ptr + field_struct.Offset))) = int16(ii)
			case reflect.Int32:
				ii, _ := strconv.Atoi(*(*string)(unsafe.Pointer(&rows.buffer)))
				*((*int32)(unsafe.Pointer(uint_ptr + field_struct.Offset))) = int32(ii)
			case reflect.Int64:
				ii, _ := strconv.Atoi(*(*string)(unsafe.Pointer(&rows.buffer)))
				*((*int64)(unsafe.Pointer(uint_ptr + field_struct.Offset))) = int64(ii)
			case reflect.Uint:
				ii, _ := strconv.Atoi(*(*string)(unsafe.Pointer(&rows.buffer)))
				*((*uint)(unsafe.Pointer(uint_ptr + field_struct.Offset))) = uint(ii)
			case reflect.Uint8:
				ii, _ := strconv.Atoi(*(*string)(unsafe.Pointer(&rows.buffer)))
				*((*uint8)(unsafe.Pointer(uint_ptr + field_struct.Offset))) = uint8(ii)
			case reflect.Uint16:
				ii, _ := strconv.Atoi(*(*string)(unsafe.Pointer(&rows.buffer)))
				*((*uint16)(unsafe.Pointer(uint_ptr + field_struct.Offset))) = uint16(ii)
			case reflect.Uint32:
				ii, _ := strconv.Atoi(*(*string)(unsafe.Pointer(&rows.buffer)))
				*((*uint32)(unsafe.Pointer(uint_ptr + field_struct.Offset))) = uint32(ii)
			case reflect.Uint64:
				ii, _ := strconv.ParseUint(*(*string)(unsafe.Pointer(&rows.buffer)), 10, 64)
				*((*uint64)(unsafe.Pointer(uint_ptr + field_struct.Offset))) = uint64(ii)
			case reflect.Float32:
				f, _ := strconv.ParseFloat(*(*string)(unsafe.Pointer(&rows.buffer)), 32)
				*((*float32)(unsafe.Pointer(uint_ptr + field_struct.Offset))) = float32(f)
			case reflect.Float64:
				f, _ := strconv.ParseFloat(*(*string)(unsafe.Pointer(&rows.buffer)), 64)
				*((*float64)(unsafe.Pointer(uint_ptr + field_struct.Offset))) = f
			case reflect.String:
				if str := string(rows.buffer); str != "NULL" {
					*((*string)(unsafe.Pointer(uint_ptr + field_struct.Offset))) = str
				}

			case reflect.Bool:
				*((*bool)(unsafe.Pointer(uint_ptr + field_struct.Offset))) = rows.buffer[0] == 48
			case reflect.Struct:
				switch field_struct.Field_t.String() {
				case "time.Time":
					*((*time.Time)(unsafe.Pointer(uint_ptr + field_struct.Offset))), _ = time.ParseInLocation("2006-01-02 15:04:05", string(rows.buffer), time.Local)
				default:
					field := reflect.NewAt(field_struct.Field_t, unsafe.Pointer(uint_ptr+field_struct.Offset))
					jsoniter.Unmarshal(rows.buffer, field.Interface())

				}

			case reflect.Slice, reflect.Map:
				field := reflect.NewAt(field_struct.Field_t, unsafe.Pointer(uint_ptr+field_struct.Offset))
				jsoniter.Unmarshal(rows.buffer, field.Interface())
			case reflect.Ptr:
				if *(*string)(unsafe.Pointer(&rows.buffer)) != "NULL" {
					if len(rows.buffer) == 0 || (len(rows.buffer) == 1 && rows.buffer[0] == 0xC0) {
						continue
					}
					field := reflect.New(field_struct.Field_t.Elem())
					err := jsoniter.Unmarshal(rows.buffer, field.Interface())
					if err == nil {
						*((*uintptr)(unsafe.Pointer(uint_ptr + field_struct.Offset))) = field.Pointer()
					}
				}

			default:
				DEBUG(fmt.Sprintf("mysql.Select()反射struct写入需要处理,字段名称%s预计类型%v", string(key), field_struct.Kind))
			}

		}

	}

	return true, nil
}

/*执行sql语句
 *返回error
 *
 */
func (this *Mysql) exec(query_sql []byte, t ...*Transaction) (result bool, err error) {
	//var res *Mysql_result
	//DEBUG(string(query_sql))
	//var lastInsertId, rowsAffected int64

	if len(t) == 1 && t[0] != nil && t[0].Connect != nil {
		_, _, err = t[0].Connect.Exec(query_sql)

	} else {
	Retry:
		_, _, err = this.db.Exec(query_sql)
		if err != nil {
			if strings.Contains(err.Error(), "EOF") {
				goto Retry
			} else if strings.Contains(err.Error(), "broken pipe") { //unix断连
				goto Retry
			} else {
				return false, err
			}
		}

	}
	result = false
	if err == nil {
		result = true
	}
	return
}

//执行语句并取受影响行数
func (this *Mysql) Query_getaffected(query_sql []byte, t *Transaction) (rowsAffected int64, err error) {

	if t != nil && t.Connect != nil {
		_, rowsAffected, err = t.Connect.Exec(query_sql)
	} else {
	Retry:
		_, rowsAffected, err = this.db.Exec(query_sql)
		if err != nil {
			if strings.Contains(err.Error(), "EOF") {
				goto Retry
			} else if strings.Contains(err.Error(), "broken pipe") { //unix断连
				goto Retry
			} else {
				return 0, err
			}
		}
	}
	return
}

/*列出所有表
func (this *Mysql) ShowTables(master string) (list orm.ParamsList) {
	if master != "slave" && master != "default" {
		master = "default"
	}
	s := o
	s.Using(master)
	sql := "SHOW TABLES"
	s.Raw(sql).ValuesFlat(&list)
	return
}*/

/*列出表结构
func (this *Mysql) ShowColumns(table string, master string) map[string]Mysql_columns {
	sql := "SHOW COLUMNS FROM `" + table + "`"
	result, err := this.Select(sql, master, new(Transaction))
	Errorlog(err, "初始化错误，无法列出表结构")
	re := make(map[string]Mysql_columns)
	for _, tmp := range result {
		re[tmp["Field"].(string)] = Mysql_columns{Name: tmp["Field"].(string), Sql_type: tmp["Type"].(string), Null: tmp["Null"].(string), Sql_default: tmp["Default"], Primary: (tmp["Key"].(string) == "PRI"), Autoinc: (tmp["Extra"].(string) == "auto_increment")}
	}
	return re
}*/

//开始事务
func (this *Mysql) NewSession() (t *Transaction) {
	t = &Transaction{DB: this.db, Mysql_build: new(Mysql_build)}
	j := &Json_encode{}
	j.B = bytes.NewBuffer(nil)
	j.E = gjson.NewEncoder(j.B)
	t.Mysql_build.json_encode = j
	t.Mysql_build.DB = this
	return
}
func (t *Transaction) Begin() (err error) {
	t.Connect, err = t.DB.BeginTransaction()
	if err != nil {
		return
	}
	t.Mysql_build.t = t
	return
}

//提交事务
func (t *Transaction) Commit() error {
	_, _, err := t.Connect.Exec([]byte{99, 111, 109, 109, 105, 116})
	return err
}

//回滚事务
func (t *Transaction) Rollback() error {
	_, _, err := t.Connect.Exec([]byte{114, 111, 108, 108, 98, 97, 99, 107})
	return err
}
func (t *Transaction) Close() {
	if t.Connect != nil && t != nil {
		conn := t.Connect
		if err := recover(); err != nil {
			debug.PrintStack()
		}
		t.Connect.Status = false
		t.Connect = nil
		//rollback
		conn.Exec([]byte{114, 111, 108, 108, 98, 97, 99, 107})
		t.DB.EndTransaction(conn)
	}
}
func (mysql *Mysql) Close() {
	mysql.db.Conn_m.Range(func(k, v interface{}) bool {
		v.(*Mysql_Conn).Close()
		v.(*Mysql_Conn).Status = false
		mysql.db.Conn_m.Delete(k)
		return true
	})
}
func Mysql_init(db string, maxConn int, maxIdle int, maxLife int) (mysql *Mysql, err error) {
	mysql = new(Mysql)
	var str [][]string

	if str, _ = Preg_match_result(`([^:]+):([^@]+)@(tcp)?(unix)?\(([^)]*)\)\/([^?]+)\?charset=(\S+)`, db, 1); str == nil || len(str[0]) != 8 {
		return nil, errors.New("连接数据库，无法解析连接字串：" + db)
	}
	var charset = "utf8"
	_, offset := time.Now().Zone()
	var time_zone string
	if offset >= 0 {
		time_zone = "+" + strconv.Itoa(offset/3600) + ":00"
	} else {
		time_zone = strconv.Itoa(offset/3600) + ":00"
	}
	if str[0][7] != "" {
		for _, s := range strings.Split(str[0][7], "&") {
			if value := strings.Split(url.PathEscape(s), "="); len(value) == 2 {
				switch value[0] {
				case "charset":
					charset = value[1]
				case "time_zone":
					time_zone = value[2]
				}
			}
		}
	}
	mysql.db = mysql_open(str[0][1], str[0][2], str[0][5], str[0][6], charset, time_zone, nil)
	mysql.db.MaxOpenConns = int32(maxConn)
	if maxIdle <= 0 {
		maxIdle = 1
	}
	mysql.db.MaxIdleConns = int32(maxIdle)
	mysql.db.ConnMaxLifetime = int64(maxLife)
	err = mysql.db.Ping()
	build_chan = make(chan *Mysql_build, maxConn)
	for i := 0; i < maxConn; i++ {
		b := &Mysql_build{}
		b.str.Grow(1024)
		b.table_name.Grow(1024)
		b.where.Grow(1024 * 10)
		b.group.Grow(256)
		b.order.Grow(256)
		b.limit.Grow(256)
		b.field.Grow(1024 * 10)
		j := &Json_encode{}
		j.B = bytes.NewBuffer(nil)
		j.E = gjson.NewEncoder(j.B)
		b.json_encode = j
		build_chan <- b

	}
	return
}

var build_chan chan *Mysql_build

func (mysql *Mysql) Sync2(i ...interface{}) (errs []error) {

	res, err := mysql.QueryString("select version()")
	if err != nil {

		return []error{err}
	}
	is_mariadb := strings.Contains(res[0]["version()"], "MariaDB")
	var default_engine string
	var support_tokudb bool
	res, err = mysql.QueryString("show engines")
	for _, v := range res {
		if v["Support"] == "DEFAULT" {
			default_engine = v["Engine"]
		}
		if v["Engine"] == "TokuDB" {
			support_tokudb = (v["Support"] == "DEFAULT" || v["Support"] == "YES")
		}

	}
	if mysql.storeEngine == "" {
		mysql.storeEngine = default_engine
	}
	if mysql.storeEngine == "Archive" { //mariadb不支持Archive
		if is_mariadb && support_tokudb {
			mysql.storeEngine = "TokuDB"
		} else if is_mariadb {
			mysql.storeEngine = "MyISAM"
		}
	} else if mysql.storeEngine == "TokuDB" { //mysql不支持TokuDB
		if !support_tokudb {
			mysql.storeEngine = "MyISAM"
		}
	}

	var wg sync.WaitGroup
	wg.Add(len(i))
	for _, v := range i {
		go func(v interface{}) {
			defer wg.Done()
			buf := bytes.NewBuffer(nil)
			buf2 := bytes.NewBuffer(nil)
			obj := reflect.ValueOf(v)
			if obj.Kind() != reflect.Ptr {
				errs = append(errs, errors.New("sync2需要传入指针型struct"))
				return
			}
			r := obj.Elem()
			t := r.Type()
			table_name := t.Name()

			res, err := mysql.QueryString(`show tables like ?`, table_name)
			if err != nil {
				errs = append(errs, errors.New(table_name+":"+err.Error()))
				return
			}

			index := map[string]bool{} //普通索引
			if res == nil {
				buf.Reset()
				buf.WriteString("CREATE TABLE `")
				buf.WriteString(table_name)
				buf.WriteString("` (")
				buf2.Reset()
				buf2.WriteString("PRIMARY KEY (")
				var have_pk bool

				for i := 0; i < r.NumField(); i++ {
					var is_pk, notnull bool
					var default_str string
					field := r.Field(i)
					field_t := t.Field(i)
					field_str := field_t.Name
					tags := field_t.Tag.Get(`xorm`)
					if tags == `-` {
						continue
					}
					if strings.Contains(tags, "pk") {
						is_pk = true
						have_pk = true
						buf2.WriteString("`" + field_str + "`")
						buf2.WriteByte(44)
						notnull = true
					}
					if strings.Contains(tags, "notnull") || strings.Contains(tags, "not null") {
						notnull = true
					}
					if strings.Contains(tags, `index`) {
						index[field_str] = true
						notnull = true
					}

					if sc, _ := Preg_match_result(`default\((\d+)\)`, tags, 1); len(sc) > 0 {
						default_str = " DEFAULT '" + sc[0][1] + "'"
					} else if sc, _ := Preg_match_result(`default\('([^']*)'\)`, tags, 1); len(sc) > 0 {
						default_str = " DEFAULT '" + sc[0][1] + "'"
					}
					buf.WriteString("`" + field_str + "` ")
					var is_text bool
					switch field.Kind() {
					case reflect.Int64, reflect.Uint64, reflect.Int:
						buf.WriteString("bigint(20)")
						if default_str == "" {
							default_str = " DEFAULT '0'"
						}
					case reflect.String:

						if sc, _ := Preg_match_result(`varchar\(\d+\)`, tags, 1); len(sc) > 0 {
							buf.WriteString(sc[0][0])
							if default_str == "" {
								default_str = " DEFAULT ''"
							}
							break
						}
						if is_pk {
							buf.WriteString(`varchar(255)`)
							if default_str == "" {
								default_str = " DEFAULT ''"
							}
							break
						}
						is_text = true
						buf.WriteString("text")
					case reflect.Int32, reflect.Uint32:
						buf.WriteString("int(11)")
						if default_str == "" {
							default_str = " DEFAULT '0'"
						}
					case reflect.Int8, reflect.Uint8:
						buf.WriteString("tinyint(3)")
						if default_str == "" {
							default_str = " DEFAULT '0'"
						}
					case reflect.Int16, reflect.Uint16:
						buf.WriteString("smallint(6)")
						if default_str == "" {
							default_str = " DEFAULT '0'"
						}
					case reflect.Float32:
						buf.WriteString("float")
						if default_str == "" {
							default_str = " DEFAULT 0"
						}
					case reflect.Bool:
						buf.WriteString("tinyint(1)")
						if default_str == "" {
							default_str = " DEFAULT '0'"
						}
					case reflect.Struct:
						switch field.Interface().(type) {
						case time.Time:
							buf.WriteString("datetime")
							default_str = " DEFAULT current_timestamp()"
						default:

							is_text = true
							switch {
							case strings.Contains(tags, "longblob"):
								buf.WriteString("longblob")
							case strings.Contains(tags, "mediumblob"):
								buf.WriteString("mediumblob")
							case strings.Contains(tags, "tinyblob"):
								buf.WriteString("tinyblob")
							default:
								buf.WriteString("blob")
							}

						}
					default:
						is_text = true
						switch {
						case strings.Contains(tags, "mediumblob"):
							buf.WriteString("MediumBlob")
						case strings.Contains(tags, "longblob"):
							buf.WriteString("longblob")
						case strings.Contains(tags, "tinyblob"):
							buf.WriteString("tinyblob")
						default:
							buf.WriteString("blob")
						}

					}
					if is_pk {
						buf.WriteString(" NOT NULL")
						if strings.Contains(tags, "auto_increment") {
							buf.WriteString(" AUTO_INCREMENT")
						} else {
							buf.WriteString(default_str)
						}

						buf.WriteByte(44)
						continue
					}

					if notnull {
						buf.WriteString(" NOT NULL")
					} else {
						buf.WriteString(" NULL")
					}
					if strings.Contains(tags, "auto_increment") {
						buf.WriteString(" AUTO_INCREMENT")
					} else if !is_text {
						buf.WriteString(default_str)
					}
					buf.WriteByte(44)
				}
				if have_pk {
					buf.Write(buf2.Next(buf2.Len() - 1))
					buf.WriteString(")")
				} else {
					l := buf.Len()
					buf.Write(buf.Next(l)[:l-1])
				}
				buf.WriteString(") ENGINE=")
				buf.WriteString(mysql.storeEngine)
				buf.WriteString(" DEFAULT CHARSET=utf8")
				_, err := mysql.exec(buf.Bytes(), nil)
				if err != nil {
					errs = append(errs, errors.New("执行新建数据库失败："+err.Error()+" 错误sql:"+buf.String()))
					return
				}
			} else {
				res, err = mysql.QueryString(`desc ` + table_name)
				if err != nil {
					errs = append(errs, errors.New(table_name+":"+err.Error()))
					return
				}
				var pk, sql []string
				var pk_num int
				var res_m = make(map[string]map[string]string, len(res))
				for _, value := range res {
					if value["Key"] == "PRI" {
						pk_num++
					}
					res_m[value["Field"]] = value
				}

				for i := 0; i < r.NumField(); i++ {
					field_t := t.Field(i)
					field := r.Field(i)
					tag := field_t.Tag.Get(`xorm`)
					if tag == `-` {
						continue
					}
					field_str := field_t.Name
					var is_change int8
					var is_text bool
					var notnull, is_pk bool
					var default_str, varchar_str, extra_str string
					sql_str := make([]string, 5)
					if value, ok := res_m[field_str]; ok {
						extra_str = ""
						sql_str[4] = value["Extra"]
						default_str = value["Default"]
						sql_str[1] = value["Type"]
						if value["Null"] == "YES" {
							sql_str[2] = "NULL"
						} else {
							sql_str[2] = "NOT NULL"
						}

						sql_str[3] = value["Default"]
						if sql_str[3] == "''" {
							sql_str[3] = ""
						}
						if default_str == "''" {
							default_str = ""
						}
						if strings.Contains(tag, "pk") {
							is_pk = true
							notnull = true
						}
						if strings.Contains(tag, "notnull") || strings.Contains(tag, "not null") {
							notnull = true
						}
						if strings.Contains(tag, "index") {
							index[field_str] = true
							notnull = true
							if sql_str[2] == "NULL" {
								sql_str[2] = "NOT NULL"
							}
						}

						if sc, _ := Preg_match_result(`default\((\d+)\)`, tag, 1); len(sc) > 0 {
							default_str = sc[0][1]

						} else if sc, _ := Preg_match_result(`default\('([^']*)'\)`, tag, 1); len(sc) > 0 {
							default_str = sc[0][1]
						}
						if sc, _ := Preg_match_result(`extra\('([^']*)'\)`, tag, 1); len(sc) > 0 {
							extra_str = sc[0][1]
						}

						switch {
						case strings.Contains(tag, "longblob"):
							varchar_str = "longblob"
							notnull = false
						case strings.Contains(tag, "mediumblob"):
							varchar_str = "mediumblob"
							notnull = false
						case strings.Contains(tag, "tinyblob"):
							varchar_str = "tinyblob"
							notnull = false
						case strings.Contains(tag, "blob"):
							varchar_str = "blob"
							notnull = false
						case strings.Contains(tag, "longtext"):
							varchar_str = "longtext"
							notnull = false
						case strings.Contains(tag, "mediumtext"):
							varchar_str = "mediumtext"
							notnull = false
						case strings.Contains(tag, "tinytext"):
							varchar_str = "tinytext"
							notnull = false
						case strings.Contains(tag, "text") && !strings.Contains(tag, "'text'"):
							varchar_str = "text"
							notnull = false
						default:
							if sc, _ := Preg_match_result(`varchar\(\d+\)`, tag, 1); len(sc) > 0 {
								varchar_str = sc[0][0]
							}
						}

						if notnull {
							if value["Null"] == "YES" {
								is_change = 1
								sql_str[2] = "NOT NULL"
							}

						} else {
							if value["Null"] == "NO" {
								is_change = 2
								sql_str[2] = "NULL"
							}
						}
						if strings.Contains(tag, "auto_increment") {
							extra_str = "auto_increment"
							if !strings.Contains(value["Extra"], "auto_increment") {
								is_change = 3
							}
						}

						switch field.Kind() {
						case reflect.Int64, reflect.Int:
							if sql_str[1] != "bigint(20)" && sql_str[1] != "bigint" {
								is_change = 4
								sql_str[1] = "bigint(20)"
							}
							if default_str == "" || (is_pk && default_str == "NULL") {
								default_str = "0"
							}
						case reflect.Uint64, reflect.Uint:
							if sql_str[1] != "bigint(20) unsigned" {
								is_change = 5
								sql_str[1] = "bigint(20) unsigned"
							}
							if default_str == "" || (is_pk && default_str == "NULL") {
								default_str = "0"
							}
						case reflect.Float32:
							if sql_str[1] != "float" {
								is_change = 6
								sql_str[1] = "float"
							}
							if default_str == "" || (is_pk && default_str == "NULL") {
								default_str = "0"
							}
						case reflect.Float64:
							if sql_str[1] != "double" {
								is_change = 6
								sql_str[1] = "double"
							}
							if default_str == "" || (is_pk && default_str == "NULL") {
								default_str = "0"
							}
						case reflect.String:
							if varchar_str != "" {
								if sql_str[1] != varchar_str {
									is_change = 7
									sql_str[1] = varchar_str
								}
								break
							}
							sql_str[3] = default_str
							if strings.Contains(tag, "type:text") {
								is_text = true
								if is_pk {
									if sql_str[1] != text_pk_type_str {
										is_change = 8
										sql_str[1] = "text"
									}
								} else {
									if sql_str[1] != "text" {
										is_change = 9
										sql_str[1] = "text"
									}
								}
							} else {
								is_text = true
								if is_pk {
									if sql_str[1] != text_pk_type_str {
										is_change = 10
										sql_str[1] = "text"
									}
								} else {
									if sql_str[1] != "text" {
										is_change = 11
										sql_str[1] = "text"
									}
								}

							}
						case reflect.Int32:
							if sql_str[1] != "int(11)" && sql_str[1] != "int" {
								is_change = 12
								sql_str[1] = "int(11)"
							}
							if default_str == "" || (is_pk && default_str == "NULL") {
								default_str = "0"
							}

						case reflect.Uint32:
							if sql_str[1] != "int(11) unsigned" {
								is_change = 13
								sql_str[1] = "int(11) unsigned"
							}
							if default_str == "" || (is_pk && default_str == "NULL") {
								default_str = "0"
							}
						case reflect.Int8:
							if sql_str[1] != "tinyint(3)" && sql_str[1] != "tinyint" {
								is_change = 14
								sql_str[1] = "tinyint(3)"
							}
							if default_str == "" || (is_pk && default_str == "NULL") {
								default_str = "0"
							}
						case reflect.Uint8:
							if sql_str[1] != "tinyint(3) unsigned" {
								is_change = 15
								sql_str[1] = "tinyint(3) unsigned"
							}
							if default_str == "" || (is_pk && default_str == "NULL") {
								default_str = "0"
							}
						case reflect.Int16:
							if sql_str[1] != "smallint(6)" && sql_str[1] != "smallint" {
								is_change = 16
								sql_str[1] = "smallint(6)"
							}
							if default_str == "" || (is_pk && default_str == "NULL") {
								default_str = "0"
							}
						case reflect.Uint16:
							if sql_str[1] != "smallint(6) unsigned" {
								is_change = 17
								sql_str[1] = "smallint(6) unsigned"
							}
							if default_str == "" || (is_pk && default_str == "NULL") {
								default_str = "0"
							}
						case reflect.Bool:
							if sql_str[1] != "tinyint(1)" {
								is_change = 18
								sql_str[1] = "tinyint(1)"
							}
							if default_str == "" || (is_pk && default_str == "NULL") {
								default_str = "1"
							}
						case reflect.Struct:
							switch field.Interface().(type) {
							case time.Time:
								var timestr = "datetime"
								switch {
								case strings.Contains(tag, "type:timestamp"):
									timestr = "timestamp"
								case strings.Contains(tag, "type:time"):
									timestr = "time"
								case strings.Contains(tag, "type:date"):
									timestr = "date"
								}
								if sql_str[1] != timestr {
									is_change = 19
									sql_str[1] = timestr
								}
								if Preg_match(`^\d{4}-\d{2}-\d{2}$`, default_str) {
									default_str += " 00:00:00"
								}
								if default_str == "" || default_str == "NULL" {
									default_str = "current_timestamp()"
								}
								if sql_str[4] == "DEFAULT_GENERATED" && extra_str == "" {
									extra_str = sql_str[4]
								}
							default:
								is_text = true
								if !strings.Contains(sql_str[1], "text") {
									is_change = 20
									sql_str[1] = "text"
								}
								default_str = "NULL"
							}
						default:

							if varchar_str != "" {
								if sql_str[1] != varchar_str {
									is_change = 21
									sql_str[1] = varchar_str
								}

							} else {
								is_text = true
								if !strings.Contains(sql_str[1], "text") {
									is_change = 22
									sql_str[1] = "text"
								}

							}

							if default_str == "" && !notnull {
								default_str = "NULL"
							}
						}
						if is_pk {
							pk = append(pk, field_str)
							sql_str[2] = "NOT NULL"
							if !strings.Contains(sql_str[1], "char") {
								if sql_str[3] != "0" && sql_str[3] != "current_timestamp()" {
									sql_str[3] = "NULL"
								}
								if extra_str == "auto_increment" || default_str == "" {
									default_str = "NULL"
								}
							}

							if is_text {
								sql_str[1] = text_pk_type_str
							}

						}
						if sql_str[3] != default_str {
							DEBUG(sql_str[3], default_str)
							is_change = 23
							sql_str[3] = default_str
						}
						if sql_str[4] != extra_str {

							is_change = 24
							sql_str[4] = extra_str
						}
						if sql_str[3] != "" {
							switch sql_str[3] {
							case "current_timestamp()", "CURRENT_TIMESTAMP":
								sql_str[3] = "Default " + sql_str[3]
							case "AUTO_INCREMENT":
							case "NULL":
								sql_str[3] = "Default NULL"
							default:
								sql_str[3] = "Default '" + strings.Trim(sql_str[3], "'") + "'"
							}

						} else {
							sql_str[3] = "Default ''"
						}

						if is_change > 0 {
							if is_text {
								sql_str[3] = ""
							}
							sql_str[0] = "modify column `" + field_str + "`"
							DEBUG(is_change, sql_str)
							sql = append(sql, strings.Join(sql_str, " "))
						}
					} else {

						var after string
						if i == 0 {
							after = " FIRST"
						}
						for index := i - 1; index > -1; index-- {
							before_field := t.Field(index)
							if before_field.Tag.Get(`xorm`) == `-` {
								continue
							}
							after = " AFTER `" + before_field.Name + "`"
							break
						}

						switch field.Kind() {
						case reflect.Int64, reflect.Int:
							sql_str[1] = "bigint(20)"
							sql_str[3] = "Default '0'"
						case reflect.Uint64, reflect.Uint:
							sql_str[1] = "bigint(20) unsigned"
							sql_str[3] = "Default '0'"
						case reflect.String:

							sql_str[3] = "Default ''"
							if varchar_str != "" {
								sql_str[1] = varchar_str
								sql_str[3] = "Default ''"
								break
							}
							is_text = true
							sql_str[1] = "text"
						case reflect.Int32:
							sql_str[1] = "int(11)"
							sql_str[3] = "Default '0'"
						case reflect.Uint32:
							sql_str[1] = "int(11) unsigned"
							sql_str[3] = "Default '0'"
						case reflect.Int8:
							sql_str[1] = "tinyint(3)"
							sql_str[3] = "Default '0'"
						case reflect.Uint8:
							sql_str[1] = "tinyint(3) unsigned"
							sql_str[3] = "Default '0'"
						case reflect.Int16:
							sql_str[1] = "smallint(6)"
							sql_str[3] = "Default '0'"
						case reflect.Uint16:
							sql_str[1] = "smallint(6) unsigned"
							sql_str[3] = "Default '0'"

						case reflect.Bool:
							sql_str[1] = "tinyint(1)"
							sql_str[3] = "Default '0'"
						case reflect.Struct:
							switch r.Field(i).Interface().(type) {
							case time.Time:
								switch {
								case strings.Contains(tag, "type:timestamp"):
									sql_str[1] = "timestamp"
								case strings.Contains(tag, "type:time"):
									sql_str[1] = "time"
								case strings.Contains(tag, "type:date"):
									sql_str[1] = "date"
								default:
									sql_str[1] = "datetime"
								}

								sql_str[3] = "Default current_timestamp()"
							default:
								is_text = true
								sql_str[1] = "text"
							}
						default:
							is_text = true
							sql_str[1] = "text"
						}
						if strings.Contains(tag, "auto_increment") {
							if !strings.Contains(value["Extra"], "auto_increment") {
								sql_str[3] = " AUTO_INCREMENT"
							}
						}
						switch {
						case strings.Contains(tag, "type:longblob"):
							sql_str[1] = "longblob"
						case strings.Contains(tag, "type:mediumblob"):
							sql_str[1] = "mediumblob"
						case strings.Contains(tag, "type:tinyblob"):
							sql_str[1] = "tinyblob"
						case strings.Contains(tag, "type:blob"):
							sql_str[1] = "blob"
						case strings.Contains(tag, "type:longtext"):
							sql_str[1] = "longtext"
						case strings.Contains(tag, "type:mediumtext"):
							sql_str[1] = "mediumtext"
						case strings.Contains(tag, "type:tinytext"):
							sql_str[1] = "tinytext"
						case strings.Contains(tag, "type:text"):
							sql_str[1] = "text"

							sql_str[3] = strings.Replace(sql_str[3], " Default NULL", "", 1)
						default:
							if sc, _ := Preg_match_result(`type:(varchar\(\d+\))`, tag, 1); len(sc) > 0 {
								sql_str[1] = sc[0][1]
							} else {
								if sc, _ := Preg_match_result(`type:(char\(\d+\))`, tag, 1); len(sc) > 0 {
									sql_str[1] = sc[0][1]
								}
							}
						}
						if strings.Contains(tag, "notnull") || strings.Contains(tag, "not null") {
							notnull = true
						}
						if strings.Contains(tag, "pk") {
							pk = append(pk, field_str)
							sql_str[2] = "NOT NULL"
							if sql_str[3] != " AUTO_INCREMENT" {
								sql_str[3] = ""
							}
							if sql_str[1] == "text" {
								sql_str[1] = text_pk_type_str
							}
						} else {

							if sc, _ := Preg_match_result(`default\((\d+)\)`, tag, 1); len(sc) > 0 && !is_text {
								sql_str[3] = "Default '" + sc[0][1] + "'"

							}

							if sc, _ := Preg_match_result(`default\('([^']*)'\)`, tag, 1); !is_text && len(sc) > 0 {
								switch sc[0][1] {
								case "current_timestamp()":
									sql_str[3] = "Default " + sc[0][1]
								case "NULL":
									sql_str[3] = "Default NULL"
								default:
									sql_str[3] = "Default '" + strings.Trim(sc[0][1], "'") + "'"
								}

							}
							if sc, _ := Preg_match_result(`extra\('([^']*)'\)`, tag, 1); len(sc) > 0 {
								sql_str[4] = sc[0][1]
							}
							if notnull {
								sql_str[2] = "NOT NULL"
							} else {
								sql_str[2] = "NULL"
							}
						}

						sql_str[0] = "ADD `" + field_str + "`"
						sql = append(sql, strings.Join(sql_str, " ")+after)
					}
				}
				if pk_num != len(pk) {
					if pk_num == 0 {
						sql = append(sql, "ADD PRIMARY KEY (`"+strings.Join(pk, "`,`")+"`)")
					} else if len(pk) == 0 {
						sql = append(sql, "DROP PRIMARY KEY")
					} else {
						sql = append(sql, "DROP PRIMARY KEY,ADD PRIMARY KEY (`"+strings.Join(pk, "`,`")+"`)")
					}
				}
				if len(sql) > 0 {
					s := "ALTER TABLE " + table_name + " " + strings.Join(sql, ",")
					DEBUG(s)
					_, err := mysql.exec(Str2bytes(s), nil)

					if err != nil {
						errs = append(errs, errors.New(table_name+":"+err.Error()))
						return
					}
				}

				res, err := mysql.QueryString("select ENGINE from information_schema.tables where table_name=? and TABLE_SCHEMA=?", table_name, mysql.db.database)
				if err != nil {
					errs = append(errs, errors.New(table_name+":"+err.Error()))
					return
				}
				if res[0]["ENGINE"] != mysql.storeEngine {
					_, err := mysql.exec([]byte("ALTER TABLE "+table_name+" ENGINE = "+mysql.storeEngine), nil)
					if err != nil {
						errs = append(errs, errors.New(table_name+":"+err.Error()))
						return
					}
				}
			}
			if len(index) > 0 {

				res, err = mysql.QueryString(`show index from ` + table_name)
				if err != nil {
					errs = append(errs, errors.New(table_name+":"+err.Error()))
					return
				}
				keys := map[string]bool{}
				for _, v := range res {
					if v["Key_name"] == "PRIMARY" {
						continue
					}
					keys[v["Key_name"]] = true
				}
				for k := range index {
					if !keys[k] {
						buf.Reset()
						buf.WriteString("ALTER TABLE ")
						buf.WriteString(table_name)
						buf.WriteString(" ADD INDEX ")
						buf.WriteString(k)
						buf.WriteString(" (`")
						buf.WriteString(k)
						buf.WriteString("`)")
						_, err = mysql.exec(buf.Bytes(), nil)
						if err != nil {
							errs = append(errs, errors.New(table_name+":"+err.Error()))
							return
						}
					}
				}
			}

		}(v)
	}

	wg.Wait()
	return errs
}

func (mysql *Mysql) StoreEngine(storeEngine string) *Mysql {
	new_mysql := &Mysql{db: mysql.db, storeEngine: storeEngine}
	return new_mysql
}
