package libraries

import (
	"reflect"
	"sync"
)

var rows_pool = sync.Pool{New: func() interface{} {
	row := &MysqlRows{Buffer: new(MsgBuffer), Buffer2: new(MsgBuffer), field_m: make(map[string]map[string]*Field_struct)}

	return row
}}

type Field_struct struct {
	Offset              uintptr
	ColumnName, KeyName string

	Field_t reflect.Type
}
type MysqlRows struct {
	Buffer, Buffer2 *MsgBuffer
	field_len       int
	msg_len         []int
	buffer          []byte
	//msg_buffer_no *int
	field      []byte
	field_m    map[string]map[string]*Field_struct
	fields     []MysqlColumn
	result_len int
	IsBinary   bool
	conn       *Mysql_Conn
}
type MysqlColumn struct {
	name      []byte
	fieldtype fieldType
	fleldflag fieldFlag
	decimals  uint8
}

func (row *MysqlRows) Columns(mysql *Mysql_Conn) (columns []MysqlColumn, err error) {
	if cap(row.fields) < row.field_len {
		row.fields = make([]MysqlColumn, 0, row.field_len)
	}
	row.result_len = 0
	columns = row.fields[:row.field_len]
	var index uint32
	var msglen, pos, field_index int
	var data []byte
	for data, err = mysql.readOneMsg(); err == nil && !(len(data) == 5 && data[0] == 0xfe); data, err = mysql.readOneMsg() {
		pos = 0
		msglen, err = ReadLength_Coded_Slice(data, &pos)
		if err != nil {
			return
		}
		pos += msglen

		// Database [len coded string]
		msglen, err = ReadLength_Coded_Slice(data[pos:], &pos)
		if err != nil {
			return
		}

		pos += msglen
		// Table [len coded string]
		msglen, err = ReadLength_Coded_Slice(data[pos:], &pos)
		if err != nil {
			return
		}
		pos += msglen
		// Original table [len coded string]
		msglen, err = ReadLength_Coded_Slice(data[pos:], &pos)
		if err != nil {
			return
		}
		pos += msglen
		// Name [len coded string]
		msglen, err = ReadLength_Coded_Slice(data[pos:], &pos)
		if err != nil {
			return
		}
		if field_index+msglen > len(row.field) {
			row.field = append(row.field, make([]byte, field_index+msglen-len(row.field))...)
		}
		columns[index].name = row.field[field_index : field_index+msglen]
		copy(columns[index].name, data[pos:pos+msglen])
		field_index += msglen
		if row.IsBinary {
			pos += msglen
			msglen, err = ReadLength_Coded_Slice(data[pos:], &pos)
			if err != nil {
				return
			}
			pos += msglen
			pos += 7
			columns[index].fieldtype = fieldType(data[pos])
			columns[index].fleldflag = fieldFlag(uint16(data[pos+1]) + uint16(data[pos+2])<<8)
			columns[index].decimals = data[pos+3]
		}
		index++

	}
	//libraries.DEBUG(row.Buffer.Bytes())
	if err != nil {
		return
	}
	row.Buffer.Reset()
	row.msg_len = row.msg_len[:0]
	for data, err = mysql.readOneMsg(); err == nil && !(len(data) == 5 && data[0] == 0xfe); data, err = mysql.readOneMsg() {
		row.Buffer.Write(data)
		row.result_len++
		row.msg_len = append(row.msg_len, len(data))
	}

	return columns, err
}

func (row *MysqlRows) Scan(a ...*[]byte) error {
	var err error
	for _, v := range a {
		*v, err = ReadLength_Coded_Byte(row.Buffer)
		if err != nil {
			return err
		}
	}
	return nil

}
