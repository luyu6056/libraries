package libraries

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"

	//"protocol"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"
)

var INSERT_TYPE = "replace into "

type Mysql_build struct {
	field       bytes.Buffer //字段
	table_name  bytes.Buffer
	where       bytes.Buffer
	DB          *Mysql
	t           *Transaction
	group       bytes.Buffer
	order       bytes.Buffer
	lock        bytes.Buffer
	limit       bytes.Buffer
	pk          PK
	str         bytes.Buffer
	err         error
	json_encode *Json_encode
}
type PK []interface{}

func NewPK(pks ...interface{}) *PK {
	p := PK(pks)
	return &p
}

var Field_build sync.Map

var key_srp = strings.NewReplacer(`\`, `\\`, "`rank`", "rank", "`type`", "type", "`", "\\`", string(rune(0)), `\`+string(rune(0)))
var value_srp = strings.NewReplacer(`\`, `\\`, "'", `\'`, string(rune(0)), `\`+string(rune(0)))

func (this *Mysql) Where(format interface{}, i ...interface{}) *Mysql_build {
	build := <-build_chan
	build.DB = this
	build.field.WriteByte(42)
	return build.Where(format, i...)
}
func (this *Mysql_build) Where(format interface{}, i ...interface{}) *Mysql_build {
	switch format.(type) {
	case *PK:
		this.pk = *(format.(*PK))
	case string:
		if format.(string) == "" {
			return this
		}
		this.where.Write([]byte{32, 119, 104, 101, 114, 101, 32})
		str := strings.Split(strings.Replace(format.(string), "?", "'?'", -1), "?")
		if len(i)+1 != len(str) {
			this.err = errors.New("where 处理?参数数量错误")
			return this
		}
		for k, v := range i {
			this.where.WriteString(str[k])
			this.bin_getValue(v, &this.where)
		}
		this.where.WriteString(str[len(i)])
		//this.where = " where " + fmt.Sprintf(strings.Replace(format.(string), "?", `%v`, -1), str...)
	default:
		t := reflect.TypeOf(format)
		DEBUG(fmt.Sprintf("fomat格式未处理,预计格式%s", t.Name()))
	}

	return this
}
func (this *Mysql) Id(i interface{}) *Mysql_build {
	build := <-build_chan
	build.DB = this
	build.field.WriteByte(42)
	return build.Id(i)
}
func (this *Mysql) ID(i interface{}) *Mysql_build {
	build := <-build_chan
	build.DB = this
	build.field.WriteByte(42)
	return build.Id(i)
}
func (this *Mysql_build) Id(i interface{}) *Mysql_build {
	switch i.(type) {
	case *PK:
		this.pk = *(i.(*PK))
	default:
		this.pk = []interface{}{i}
	}

	return this
}
func (this *Mysql_build) ID(i interface{}) *Mysql_build {
	return this.Id(i)
}
func (this *Mysql) AllCols() *Mysql_build {
	build := <-build_chan
	build.DB = this
	build.field.WriteByte(42)
	return build
}
func (this *Mysql_build) AllCols() *Mysql_build {
	return this
}
func (this *Mysql) Cols(s ...string) *Mysql_build {
	build := <-build_chan
	build.DB = this
	return build.Cols(s...)
}
func (this *Mysql_build) Cols(s ...string) *Mysql_build {
	this.field.Reset()
	for _, v := range s {
		this.field.WriteString(v)
		this.field.WriteByte(44)
	}
	return this
}
func (this *Mysql) OrderBy(str string) *Mysql_build {
	build := <-build_chan
	build.DB = this
	build.field.WriteByte(42)
	return build.OrderBy(str)
}
func (this *Mysql_build) OrderBy(str string) *Mysql_build {
	this.order.Write([]byte{32, 111, 114, 100, 101, 114, 32, 98, 121, 32})
	this.order.WriteString(str)
	//this.order = " order by " + str
	return this
}
func (this *Mysql) GroupBy(str string) *Mysql_build {
	build := <-build_chan
	build.DB = this
	build.field.WriteByte(42)
	return build.GroupBy(str)
}
func (this *Mysql_build) GroupBy(str string) *Mysql_build {
	this.group.Write([]byte{32, 103, 114, 111, 117, 112, 32, 98, 121, 32})
	this.group.WriteString(str)
	//this.group = " group by " + str
	return this
}

func (this *Mysql) Limit(m, n int) *Mysql_build {
	build := <-build_chan
	build.DB = this
	return build.Limit(m, n)
}
func (this *Mysql_build) Limit(m, n int) *Mysql_build {
	this.limit.Write([]byte{32, 108, 105, 109, 105, 116, 32})
	this.limit.WriteString(strconv.Itoa(m))
	this.limit.WriteByte(44)
	this.limit.WriteString(strconv.Itoa(n))
	return this
}

func (this *Mysql_build) Update(i interface{}) (int64, error) {
	defer this.put()
	if this.err != nil {
		return 0, this.err
	}
	r := reflect.ValueOf(i)
	for r.Kind() == reflect.Ptr {
		r = r.Elem()
	}
	t := r.Type()
	this.table_name.WriteString(t.Name())

	var field_m *sync.Map
	if v, ok := Field_build.Load(this.table_name.String()); ok {
		field_m = v.(*sync.Map)
	} else {
		field_m = new(sync.Map)
		Field_build.Store(this.table_name.String(), field_m)
	}

	if len(this.pk) > 0 {
		this.str.Reset()
		var pk_i int
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if strings.Index(field.Tag.Get(`xorm`), "pk") > -1 {
				if pk_i < len(this.pk) {
					if v, ok := field_m.Load(i); ok {
						this.str.Write(v.([]byte))
						this.str.WriteByte(61)
					} else {
						bin := []byte(this.getkey(field.Name))
						this.str.Write(bin)
						this.str.WriteByte(61)
						field_m.Store(i, bin)
					}
					this.str.Write(this.getvalue(this.pk[pk_i]))
					this.str.Write([]byte{32, 97, 110, 100, 32}) // and
					pk_i++
				} else {
					break
				}
			}
		}
		if pk_i > 0 {
			this.where.Write([]byte{32, 119, 104, 101, 114, 101, 32})
			this.where.Write(this.str.Next(this.str.Len() - 5))
			//this.where = " where " + strings.Join(pkstr[:pk_i], " and ")
		}
	}
	this.str.Reset()
	this.str.Write([]byte{85, 80, 68, 65, 84, 69, 32})
	this.str.Write(this.table_name.Bytes())
	this.str.Write([]byte{32, 83, 69, 84, 32})
	if this.field.Bytes()[0] == 42 {
		r_ptr := r.Addr().Pointer()
		for i := 0; i < t.NumField(); i++ {
			field_t := t.Field(i)

			if field_t.Tag.Get(`xorm`) == `pk` || field_t.Tag.Get(`xorm`) == `-` {
				continue
			}
			if v, ok := field_m.Load(i); ok {
				this.str.Write(v.([]byte))
				this.str.WriteByte(61)
			} else {
				bin := []byte(this.getkey(field_t.Name))
				this.str.Write(bin)
				this.str.WriteByte(61)
				field_m.Store(i, bin)
			}

			this.str.Write(this.getvalueByUnitptr(r_ptr+field_t.Offset, field_t.Type.String(), field_t.Type))
			this.str.WriteByte(44)
		}
	} else {
		var field_str_m *sync.Map
		if v, ok := Field_M.Load(t.Name()); ok {
			field_str_m = v.(*sync.Map)
		} else {
			field_str_m = new(sync.Map)
			Field_M.Store(t.Name(), field_str_m)
		}
		fields := bytes.Split(this.field.Next(this.field.Len()-1), []byte{44})
		r_ptr := r.Addr().Pointer()
		for _, field_str := range fields {
			str := make([]byte, len(field_str))
			copy(str, field_str)
			//field_str[0] = bytes.ToUpper(field_str[:1])[0]
			var keyname string
			var field_struct *Field_struct
			if v, ok := field_str_m.Load(string(str)); ok {
				field_struct = v.(*Field_struct)
				if field_struct.Kind == reflect.Invalid {
					return 0, errors.New("update执行失败，找不到field值" + string(field_str))
				}
			} else {
				keyname = string(field_str)
				field, ok := t.FieldByName(keyname)
				if !ok {

					field_str_m.Store(string(str), &Field_struct{Kind: reflect.Invalid})
					return 0, errors.New("update执行失败，找不到field值" + string(field_str))

				}
				field_struct = &Field_struct{Offset: field.Offset, Kind: field.Type.Kind(), Field_t: field.Type}
				field_str_m.Store(string(str), field_struct)
			}

			this.str.WriteByte(96)
			this.str.Write(str)
			this.str.Write([]byte{96, 61})
			this.str.Write(this.getvalueByUnitptr(r_ptr+field_struct.Offset, field_struct.Field_t.String(), field_struct.Field_t))
			this.str.WriteByte(44)

		}
	}
	this.str.Write(this.str.Next(this.str.Len() - 1))
	this.str.Write(this.where.Bytes())
	this.str.Next(1)
	//sql := `UPDATE ` + this.table_name.String() + ` SET ` + string(this.str.Next(this.str.Len()-1)) + this.where.String()
	//fmt.Println(this.str.String())

	re, err := this.DB.Query_getaffected(this.str.Bytes(), this.t)
	if err != nil {
		err = errors.New(err.Error() + "错误sql: " + this.str.String())
	}
	return re, err
}

type emptyInterface struct {
	typ  *struct{}
	word unsafe.Pointer
}

//优化截止
func (this *Mysql) Get(s interface{}) (bool, error) {
	build := <-build_chan
	build.DB = this
	build.field.WriteByte(42)
	if s != nil {
		var t reflect.Type
		r := reflect.ValueOf(s)
		for r.Kind() == reflect.Ptr {
			r = r.Elem()
			t = r.Type()
		}
		if t.Kind() == reflect.Struct {

			for i := 0; i < t.NumField(); i++ {
				field := t.Field(i)
				if strings.Index(field.Tag.Get(`xorm`), "pk") > -1 {
					build.pk = append(build.pk, r.Field(i).Interface())
				}
			}
		}
	}

	return build.Get(s)
}
func (this *Mysql_build) Get(s interface{}) (bool, error) {
	defer this.put()
	if this.err != nil {
		return false, this.err
	}
	r := reflect.ValueOf(s)
	t := r.Type()
	for r.Kind() == reflect.Ptr {
		r = r.Elem()
		t = t.Elem()
	}
	this.table_name.WriteString(t.Name())
	var field_m *sync.Map
	if v, ok := Field_build.Load(this.table_name.String()); ok {
		field_m = v.(*sync.Map)
	} else {
		field_m = new(sync.Map)
		Field_build.Store(this.table_name.String(), field_m)
	}
	if t.Kind() == reflect.Struct {
		if len(this.pk) > 0 {
			this.str.Reset()
			var pk_i int
			for i := 0; i < t.NumField(); i++ {
				field := t.Field(i)
				if strings.Index(field.Tag.Get(`xorm`), "pk") > -1 {
					if pk_i < len(this.pk) {
						if v, ok := field_m.Load(i); ok {
							this.str.Write(v.([]byte))
							this.str.WriteByte(61)
						} else {
							bin := []byte(this.getkey(field.Name))
							this.str.Write(bin)
							this.str.WriteByte(61)
							field_m.Store(i, bin)
						}
						this.str.Write(this.getvalue(this.pk[pk_i]))
						this.str.Write([]byte{32, 97, 110, 100, 32})
						pk_i++
					} else {
						break
					}
				}
			}
			if pk_i > 0 {
				this.where.Write([]byte{32, 119, 104, 101, 114, 101, 32})
				this.where.Write(this.str.Next(this.str.Len() - 5))
			}

		}
	} else {
		return false, errors.New("Get失败，传入的不是struct")
	}
	this.str.Reset()
	this.str.Write([]byte{115, 101, 108, 101, 99, 116, 32, 42, 32, 102, 114, 111, 109, 32})
	this.str.Write(this.table_name.Bytes())
	this.str.Write(this.where.Bytes())
	this.str.Write(this.group.Bytes())
	this.str.Write(this.order.Bytes())
	this.str.Write([]byte{32, 76, 73, 77, 73, 84, 32, 49})
	this.str.Write(this.lock.Bytes())
	//sql := `select * from ` + this.table_name + this.where.String() + this.group.String() + this.order.String() + ` LIMIT 1` + this.lock.String()
	re, err := this.DB.Select(this.str.Bytes(), this.t, s)
	//fmt.Println(this.str.String())
	if err != nil {
		return false, errors.New("执行Get出错，sql:" + this.str.String() + err.Error())
	}
	return re, err
}
func (this *Mysql) Find(s interface{}) error {
	build := <-build_chan
	build.DB = this
	build.field.WriteByte(42)

	return build.Find(s)
}
func (this *Mysql_build) Find(s interface{}) (err error) {
	defer this.put()
	if this.err != nil {
		return this.err
	}
	obj := reflect.ValueOf(s)
	for obj.Kind() == reflect.Ptr {
		obj = obj.Elem()
	}
	t := obj.Type()
	if obj.Kind() == reflect.Slice {
		obj.SetLen(0)
		t = t.Elem()
		for t.Kind() == reflect.Ptr {
			t = t.Elem()
		}

		this.table_name.WriteString(t.Name())
		var field_m *sync.Map
		if v, ok := Field_build.Load(this.table_name.String()); ok {
			field_m = v.(*sync.Map)
		} else {
			field_m = new(sync.Map)
			Field_build.Store(this.table_name.String(), field_m)
		}
		//log.Debug("%+v,%+v", t.Kind(), t.Name())
		if t.Kind() == reflect.Struct {
			if len(this.pk) > 0 {
				this.str.Reset()
				var pk_i int
				for i := 0; i < t.NumField(); i++ {
					field := t.Field(i)
					if strings.Index(field.Tag.Get(`xorm`), "pk") > -1 {
						if pk_i < len(this.pk) {
							if v, ok := field_m.Load(i); ok {
								this.str.Write(v.([]byte))
								this.str.WriteByte(61)
							} else {
								bin := []byte(this.getkey(field.Name))
								this.str.Write(bin)
								this.str.WriteByte(61)
								field_m.Store(i, bin)
							}
							this.str.Write(this.getvalue(this.pk[pk_i]))
							this.str.Write([]byte{32, 97, 110, 100, 32}) // and
							pk_i++
						} else {
							break
						}
					}
				}
				if pk_i > 0 {
					this.where.Write([]byte{32, 119, 104, 101, 114, 101, 32})
					this.where.Write(this.str.Next(this.str.Len() - 5))
				}
			}
		}

	} else {
		return errors.New("Find不支持传入参数")
	}
	this.str.Reset()
	this.str.Write([]byte{115, 101, 108, 101, 99, 116, 32, 42, 32, 102, 114, 111, 109, 32})
	this.str.Write(this.table_name.Bytes())
	this.str.Write(this.where.Bytes())
	this.str.Write(this.group.Bytes())
	this.str.Write(this.order.Bytes())
	this.str.Write(this.limit.Bytes())
	this.str.Write(this.lock.Bytes())
	//sql := `select ` + this.field.String() + ` from ` + this.table_name + this.where.String() + this.group.String() + this.order.String() + this.limit.String()
	//DEBUG(`sql语句` + this.str.String())

	_, e := this.DB.Select(this.str.Bytes(), this.t, s)

	if e != nil {
		err = errors.New(`执行Find出错,sql错误信息：` + e.Error() + `,错误sql：` + this.str.String())
	}
	return
}
func (this *Mysql_build) Delete(s interface{}) (bool, error) {
	defer this.put()
	if this.err != nil {
		return false, this.err
	}
	r := reflect.ValueOf(s)
	for r.Kind() == reflect.Ptr {
		r = r.Elem()
	}
	t := r.Type()
	this.table_name.WriteString(r.Type().Name())
	var field_m *sync.Map
	if v, ok := Field_build.Load(this.table_name.String()); ok {
		field_m = v.(*sync.Map)
	} else {
		field_m = new(sync.Map)
		Field_build.Store(this.table_name.String(), field_m)
	}
	if len(this.pk) > 0 {
		this.str.Reset()
		pkstr := make([]string, len(this.pk))
		var pk_i int
		for i := 0; i < t.NumField() && i < 5; i++ {
			field := t.Field(i)
			if strings.Index(field.Tag.Get(`xorm`), "pk") > -1 {
				if pk_i < len(pkstr) {
					if v, ok := field_m.Load(i); ok {
						this.str.Write(v.([]byte))
						this.str.WriteByte(61)
					} else {
						bin := []byte(this.getkey(field.Name))
						this.str.Write(bin)
						this.str.WriteByte(61)
						field_m.Store(i, bin)
					}
					this.str.Write(this.getvalue(this.pk[pk_i]))
					this.str.Write([]byte{32, 97, 110, 100, 32}) // and
					pk_i++
				} else {
					break
				}
			}
		}
		if pk_i > 0 {
			this.where.Write([]byte{32, 119, 104, 101, 114, 101, 32})
			this.where.Write(this.str.Next(this.str.Len() - 5))
		}
	}
	this.str.Reset()
	this.str.Write([]byte{68, 69, 76, 69, 84, 69, 32, 70, 82, 79, 77, 32})
	this.str.Write(this.table_name.Bytes())
	this.str.Write(this.where.Bytes())
	this.str.Write(this.order.Bytes())
	this.str.Write(this.limit.Bytes())
	//sql := `DELETE FROM ` + this.table_name + this.where.String() + this.order.String() + this.limit.String()
	result, err := this.DB.Exec(this.str.Bytes(), this.t)
	if err != nil {
		err = errors.New(`执行Delete出错,sql错误信息：` + err.Error() + `,错误sql：` + this.str.String())
	}
	return result, err
}

/*执行sql语句
 *返回新增ID和error
 *
 */
func (this *Mysql) Insert(s ...interface{}) (LastInsertId int64, err error) {
	var t *Transaction
	switch {
	case len(s) == 0:
		return 0, errors.New("Insert没有插入参数")
	case len(s) == 2:
		switch s[1].(type) {
		case *Transaction:
			t = s[1].(*Transaction)
		default:
			return 0, errors.New("Insert参数错误")
		}

	}
	//log.Debug("%+v", s[0])
	build := <-build_chan
	defer build.put()
	sql, err := build.Build_insertsql(s[0])
	if err != nil {
		return 0, err
	}
	var rowsAffected int64
	if t != nil && t.Connect != nil {
		LastInsertId, rowsAffected, err = t.Connect.Exec(sql)
	} else {
	Retry:
		LastInsertId, rowsAffected, err = this.db.Exec(sql)
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
	if err == nil {
		if LastInsertId == 0 {
			if rowsAffected == 0 {
				err = errors.New("插入失败，影响行数为0")
			}
		}
	} else {
		err = errors.New(err.Error() + "错误sql:" + string(sql))
	}
	return
}
func (this *Mysql_build) Insert(s interface{}) (LastInsertId int64, err error) {
	defer this.put()
	if this.err != nil {
		return 0, this.err
	}
	sql, err := this.Build_insertsql(s)
	if err != nil {
		return 0, err
	}
	var rowsAffected int64
	if this.t != nil && this.t.Connect != nil {
		LastInsertId, rowsAffected, err = this.t.Connect.Exec(sql)
	} else {
	Retry:
		LastInsertId, rowsAffected, err = this.DB.db.Exec(sql)
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
	if err == nil {
		if LastInsertId == 0 {
			if rowsAffected == 0 {
				err = errors.New("插入失败，影响行数为0")
			}
		}
	} else {
		err = errors.New(err.Error() + "错误sql:" + string(sql))
	}
	return
}
func (this *Mysql_build) Build_insertsql(s interface{}) ([]byte, error) {
	r := reflect.ValueOf(s)
	for r.Kind() == reflect.Ptr {
		r = r.Elem()
	}
	switch r.Kind() {
	case reflect.Struct:
		t := r.Type()
		this.table_name.WriteString(t.Name())
		var field_m *sync.Map
		if v, ok := Field_build.Load(this.table_name.String()); ok {
			field_m = v.(*sync.Map)
		} else {
			field_m = new(sync.Map)
			Field_build.Store(this.table_name.String(), field_m)
		}
		this.str.Reset()
		this.str.Write(*(*[]byte)(unsafe.Pointer(&INSERT_TYPE)))
		this.str.Write(this.table_name.Bytes())
		this.str.Write([]byte{32, 115, 101, 116, 32})
		r_ptr := r.Addr().Pointer()
		var field_t reflect.StructField
		for i1 := 0; i1 < t.NumField(); i1++ {
			field_t = t.Field(i1)
			if field_t.Tag.Get(`xorm`) == `-` {
				continue
			}
			if v, ok := field_m.Load(i1); ok {
				this.str.Write(v.([]byte))
				this.str.WriteByte(61)
			} else {
				bin := []byte(this.getkey(field_t.Name))
				this.str.Write(bin)
				this.str.WriteByte(61)
				field_m.Store(i1, bin)
			}
			this.str.Write(this.getvalueByUnitptr(r_ptr+field_t.Offset, field_t.Type.String(), field_t.Type))

			this.str.WriteByte(44)
		}
		return this.str.Next(this.str.Len() - 1), nil
	case reflect.Slice:
		this.limit.Reset() //借用
		//field_r := []reflect.Value{}
		this.field.Reset()
		if r.Len() == 0 {
			return nil, errors.New("传入长度为0的slice")
		}
		for i := 0; i < r.Len(); i++ {
			v := r.Index(i)
			for v.Kind() == reflect.Ptr {

				v = v.Elem()
			}
			switch v.Kind() {
			case reflect.Struct:
				if this.table_name.Len() == 0 {
					this.table_name.WriteString(v.Type().Name())
				}

				t := v.Type()

				var field_t reflect.StructField
				r_ptr := v.Addr().Pointer()
				this.field.WriteByte(40)
				this.where.Reset() //借用buffer
				for i1 := 0; i1 < t.NumField(); i1++ {
					field_t = t.Field(i1)
					if field_t.Tag.Get(`xorm`) == `-` {
						continue
					}
					if i == 0 {
						//取出key的排列
						this.limit.Write(this.getkey(t.Field(i1).Name))
						this.limit.WriteByte(44)
					}
					this.where.Write(this.getvalueByUnitptr(r_ptr+field_t.Offset, field_t.Type.String(), field_t.Type))
					this.where.WriteByte(44)
				}
				this.field.Write(this.where.Bytes()[:this.where.Len()-1])
				this.field.Write([]byte{41, 44})
			default:
				return nil, errors.New(`执行InsertAll出错，不支持的slice子元素插入类型` + fmt.Sprint(v.Kind()))
			}

		}
		this.str.Reset()
		this.str.Write(*(*[]byte)(unsafe.Pointer(&INSERT_TYPE)))
		this.str.Write(this.table_name.Bytes())
		this.str.Write([]byte{32, 40})
		this.str.Write(this.limit.Bytes()[:this.limit.Len()-1])
		this.str.Write([]byte{41, 32, 86, 65, 76, 85, 69, 83, 32})

		this.str.Write(this.field.Bytes()[:this.field.Len()-1])
		//sql := `replace into ` + this.table_name.String() + ` (` + strings.Join(field, `,`) + `) VALUES ` + strings.Join(value, `,`)
		//DEBUG(this.str.String())
		return this.str.Bytes(), nil
	}

	return nil, errors.New(`执行Insert出错，不支持的插入类型` + fmt.Sprint(r.Kind()))
}
func (this *Mysql_build) getvalue(str_i interface{}) []byte {
	var str string
	switch str_i.(type) {
	case int8:
		str = strconv.Itoa(int(str_i.(int8)))
	case int:
		str = strconv.Itoa(str_i.(int))
	case int16:
		str = strconv.Itoa(int(str_i.(int16)))
	case int32:
		str = strconv.Itoa(int(str_i.(int32)))
	case int64:
		str = strconv.Itoa(int(str_i.(int64)))
	case uint:
		str = strconv.Itoa(int(str_i.(uint)))
	case uint8:
		str = strconv.Itoa(int(str_i.(uint8)))
	case uint16:
		str = strconv.Itoa(int(str_i.(uint16)))
	case uint32:
		str = strconv.Itoa(int(str_i.(uint32)))
	case uint64:
		str = strconv.Itoa(int(str_i.(uint64)))
	case []byte:
		str = encodeHex(str_i.([]byte))
	case float32, float64:
		str = fmt.Sprint(str_i)
	case bool:
		if str_i.(bool) {
			str = "0"
		} else {
			str = "1"
		}
	case string:
		str = "'" + value_srp.Replace(str_i.(string)) + "'"

	case time.Time:
		str = "'" + str_i.(time.Time).Format("2006-01-02 15:04:05") + "'"
	default:
		r := reflect.ValueOf(str_i)
		for r.Kind() == reflect.Ptr {
			r = r.Elem()
		}
		if r.Kind() == reflect.Struct || r.Kind() == reflect.Map || r.Kind() == reflect.Slice {
			this.json_encode.B.Reset()
			this.err = this.json_encode.E.Encode(str_i)
			str = encodeHex(this.json_encode.B.Bytes())
		} else {
			str = encodeHex([]byte(fmt.Sprint(str_i)))
		}

	}
	return *(*[]byte)(unsafe.Pointer(&str))
}
func (this *Mysql_build) getvalueByUnitptr(ptr uintptr, type_name string, field_t reflect.Type) []byte {
	var str string
	switch type_name {
	case "int8":
		str = strconv.Itoa(int(*((*int8)(unsafe.Pointer(ptr)))))
	case "int", "uint":
		str = strconv.Itoa(*((*int)(unsafe.Pointer(ptr))))
	case "int16":
		str = strconv.Itoa(int(*((*int16)(unsafe.Pointer(ptr)))))
	case "int32":
		str = strconv.Itoa(int(*((*int32)(unsafe.Pointer(ptr)))))
	case "int64":
		str = strconv.Itoa(int(*((*int)(unsafe.Pointer(ptr)))))
	case "uint8":
		str = strconv.Itoa(int(*((*uint8)(unsafe.Pointer(ptr)))))
	case "uint16":
		str = strconv.Itoa(int(*((*uint16)(unsafe.Pointer(ptr)))))
	case "uint32":
		str = strconv.Itoa(int(*((*uint32)(unsafe.Pointer(ptr)))))
	case "uint64":
		str = strconv.Itoa(int(*((*uint64)(unsafe.Pointer(ptr)))))
	case "[]byte":
		str = encodeHex(*((*[]byte)(unsafe.Pointer(ptr))))
	case "float32":
		str = fmt.Sprint(*((*float32)(unsafe.Pointer(ptr))))
	case "float64":
		str = fmt.Sprint(*((*float64)(unsafe.Pointer(ptr))))
	case "bool":
		if *((*bool)(unsafe.Pointer(ptr))) {
			str = "0"
		} else {
			str = "1"
		}
	case "string":
		str = "'" + value_srp.Replace(*((*string)(unsafe.Pointer(ptr)))) + "'"
	case "time.Time":
		str = "'" + (*((*time.Time)(unsafe.Pointer(ptr)))).Format("2006-01-02 15:04:05") + "'"
	default:
		r := reflect.NewAt(field_t, unsafe.Pointer(ptr)).Elem()
		for r.Kind() == reflect.Ptr {
			r = r.Elem()
		}
		if r.Kind() == reflect.Struct || r.Kind() == reflect.Map || r.Kind() == reflect.Slice {
			this.json_encode.B.Reset()
			this.err = this.json_encode.E.Encode(r.Addr().Interface())
			str = encodeHex(this.json_encode.B.Bytes())
		} else {
			if r.Kind() == reflect.Invalid {
				str = "null"
			} else {
				s := fmt.Sprint(r.Addr().Interface())
				str = encodeHex(*(*[]byte)(unsafe.Pointer(&s)))
			}

		}
	}
	return *(*[]byte)(unsafe.Pointer(&str))
}
func encodeHex(bin []byte) string {
	if len(bin) == 0 {
		return "''"
	}
	return "0x" + hex.EncodeToString(bin)
}

//处理sql语句防注入不带'
func (this *Mysql_build) getkey(str_i interface{}) []byte {
	var str string
	switch str_i.(type) {
	case string:
		str = "`" + key_srp.Replace(str_i.(string)) + "`"
	default:
		t := reflect.TypeOf(str_i)
		for t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		DEBUG("mysql处理key格式错误预计格式", t.Name())

	}

	return *(*[]byte)(unsafe.Pointer(&str))
}

func (this *Mysql_build) bin_getValue(str_i interface{}, bin *bytes.Buffer) {

	switch str_i.(type) {
	case int8:
		bin.WriteString(strconv.FormatInt(int64(str_i.(int8)), 10))
	case int:
		bin.WriteString(strconv.FormatInt(int64(str_i.(int)), 10))
	case int16:
		bin.WriteString(strconv.FormatInt(int64(str_i.(int16)), 10))
	case int32:
		bin.WriteString(strconv.FormatInt(int64(str_i.(int32)), 10))
	case int64:
		bin.WriteString(strconv.FormatInt(str_i.(int64), 10))
	case uint:
		bin.WriteString(strconv.FormatInt(int64(str_i.(uint)), 10))
	case uint8:
		bin.WriteString(strconv.FormatInt(int64(str_i.(uint8)), 10))
	case uint16:
		bin.WriteString(strconv.FormatInt(int64(str_i.(uint16)), 10))
	case uint32:
		bin.WriteString(strconv.FormatInt(int64(str_i.(uint32)), 10))
	case uint64:
		bin.WriteString(strconv.FormatUint(str_i.(uint64), 10))
	case []byte:
		bin.WriteString(encodeHex(str_i.([]byte)))
	case float32, float64:
		bin.WriteString(fmt.Sprint(str_i))
	case bool:
		if str_i.(bool) {
			bin.WriteByte(48)
			//str = "0"
		} else {
			bin.WriteByte(49)
			//str = "1"
		}
	case string:
		bin.WriteString(value_srp.Replace(str_i.(string)))

	case time.Time:
		bin.WriteString(str_i.(time.Time).Format("2006-01-02 15:04:05"))
	default:
		r := reflect.ValueOf(str_i)
		for r.Kind() == reflect.Ptr {
			r = r.Elem()
		}
		var b []byte
		if r.Kind() == reflect.Struct || r.Kind() == reflect.Map || r.Kind() == reflect.Slice {
			b = Json_pack_b(str_i)
		} else {
			b = []byte(fmt.Sprint(str_i))
		}
		bin.WriteString(encodeHex(b))
	}

}

func (this *Mysql_build) put() {
	this.err = nil
	this.table_name.Reset()
	this.where.Reset()
	this.group.Reset()
	this.order.Reset()
	this.lock.Reset()
	this.limit.Reset()
	this.field.Reset()
	this.pk = nil
	if this.t != nil {
		return
	}
	build_chan <- this
}
