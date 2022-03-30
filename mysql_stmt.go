package libraries

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

type fieldType byte

const (
	fieldTypeDecimal fieldType = iota
	fieldTypeTiny
	fieldTypeShort
	fieldTypeLong
	fieldTypeFloat
	fieldTypeDouble
	fieldTypeNULL
	fieldTypeTimestamp
	fieldTypeLongLong
	fieldTypeInt24
	fieldTypeDate
	fieldTypeTime
	fieldTypeDateTime
	fieldTypeYear
	fieldTypeNewDate
	fieldTypeVarChar
	fieldTypeBit
)
const (
	fieldTypeJSON fieldType = iota + 0xf5 //245
	fieldTypeNewDecimal
	fieldTypeEnum
	fieldTypeSet
	fieldTypeTinyBLOB
	fieldTypeMediumBLOB
	fieldTypeLongBLOB
	fieldTypeBLOB
	fieldTypeVarString
	fieldTypeString
	fieldTypeGeometry
)

type fieldFlag uint16

const (
	flagNotNULL fieldFlag = 1 << iota
	flagPriKey
	flagUniqueKey
	flagMultipleKey
	flagBLOB
	flagUnsigned
	flagZeroFill
	flagBinary
	flagEnum
	flagAutoIncrement
	flagTimestamp
	flagSet
	flagUnknown1
	flagUnknown2
	flagUnknown3
	flagUnknown4
)

var stmtbufpool = sync.Pool{New: func() interface{} {

	return new(MsgBuffer)
}}
var stmtNo uint64

type Database_mysql_stmt struct {
	query        string
	conn         *Mysql_Conn
	numInput     int
	lastInsertId int64
	rowsAffected int64
	id           uint32
}

func (conn *Mysql_Conn) Prepare(sql []byte) (*Database_mysql_stmt, error) {
	query := string(sql)
	conn.stmtMutex.RLock()
	if stmt, exists := conn.stmtCache[query]; exists {
		// must update reference counter in lock scope
		conn.stmtMutex.RUnlock()
		return stmt, nil
	}
	conn.stmtMutex.RUnlock()
	conn.stmtMutex.Lock()
	defer conn.stmtMutex.Unlock()
	if stmt, exists := conn.stmtCache[query]; exists {

		return stmt, nil
	}
	stmt := &Database_mysql_stmt{conn: conn, query: query}
	if strings.Index(query, "?") == -1 {
		stmt.numInput = -1
		return stmt, nil
	}
	var err error

	msglen := len(sql) + 1
	if msglen > max_packet_size {
		err = errors.New("消息大于最大长度" + strconv.Itoa(max_packet_size))
		return nil, err
	}
	buf := stmtbufpool.Get().(*MsgBuffer)
	defer stmtbufpool.Put(buf)
	buf.Reset()
	b := buf.Make(5 + len(sql))
	b[0] = byte(msglen)
	b[1] = byte(msglen >> 8)
	b[2] = byte(msglen >> 16)
	b[3] = 0
	b[4] = 22 //StmtPrepare
	copy(b[5:], sql)
	_, err = conn.conn.Write(b)
	if err != nil {
		return nil, err
	}

	buffer, err := conn.readOneMsg()
	if err != nil {
		return nil, err
	}
	switch buffer[0] {
	case 0: //ok报文
		stmt.id = binary.LittleEndian.Uint32(buffer[1:5])
		columnCount := binary.LittleEndian.Uint16(buffer[5:7])
		stmt.numInput = int(binary.LittleEndian.Uint16(buffer[7:9]))
		if stmt.numInput > 0 {
			for {
				buffer, err = conn.readOneMsg()
				if err != nil {
					return nil, err
				}
				if len(buffer) == 5 && buffer[0] == 0xfe {
					break
				}
			}
		}
		if columnCount > 0 {
			for {
				buffer, err = conn.readOneMsg()
				if err != nil {
					return nil, err
				}
				if len(buffer) == 5 && buffer[0] == 0xfe {
					break
				}
			}
		}

	case 255: //err报文

		var msg string
		errcode := int(buffer[1]) | int(buffer[2])<<8
		if conn.Status { //未连接成功之前
			msg = string(buffer[9:])
		} else {
			msg = string(buffer[3:])
		}
		if err != nil {

			return nil, err
		}
		return nil, errors.New(strconv.Itoa(errcode) + "-" + msg)
	default:
		return nil, errors.New("无法识别StmtPrepare报文" + strconv.Itoa(int(buffer[0])))
	}

	conn.stmtCache[query] = stmt
	return stmt, nil
}
func (stmt *Database_mysql_stmt) Exec(args []interface{}) error {

	var err error
	if stmt.numInput == -1 {
		stmt.lastInsertId, stmt.rowsAffected, err = stmt.conn.Exec(Str2bytes(stmt.query))
	} else {
		if len(args) != stmt.numInput {
			return errors.New("预处理传入的参数数量不对，语句:" + stmt.query + ",参数数量:" + strconv.Itoa(stmt.numInput))
		}
		err = stmt.Execute(args)
		if err != nil {
			return err
		}

		var errmsg string
		stmt.rowsAffected, stmt.lastInsertId, _, errmsg, err = stmt.conn.readmsg()
		if errmsg != "" {
			if strings.Contains(errmsg, "1927-Connection was killed") {
				err = errors.New("EOF")
			} else { //err报文不影响mysql的status,在这里重新包装err
				err = errors.New(errmsg)
				return err
			}
		}
	}

	return err
}
func (stmt *Database_mysql_stmt) Query(args []interface{}, row *MysqlRows) (columns []MysqlColumn, err error) {
	row.IsBinary = true
	row.conn = stmt.conn
	var errmsg string

	if stmt.numInput != -1 {

		if len(args) != stmt.numInput {
			return nil, errors.New("预处理传入的参数数量不对，语句:" + stmt.query + ",参数数量:" + strconv.Itoa(stmt.numInput))
		}

		err = stmt.Execute(args)
		if err != nil {

			return nil, err
		}
		_, _, row.field_len, errmsg, err = stmt.conn.readmsg()
		if errmsg != "" {
			err = errors.New(errmsg)
		}
		if err != nil {

			return nil, err
		}
		return row.Columns(stmt.conn)
	}

	return stmt.conn.Query(Str2bytes(stmt.query), row)
}
func (stmt *Database_mysql_stmt) Execute(args []interface{}) error {

	var err error
	conn := stmt.conn
	argbuf := stmtbufpool.Get().(*MsgBuffer)
	defer stmtbufpool.Put(argbuf)
	conn.writeBuffer.Reset()
	argbuf.Reset()
	conn.writeBuffer.Make(14)

	if len(args) > 0 {

		var nullMask []byte
		maskLen, typesLen := (len(args)+7)/8, 1+2*len(args)
		// buffer has to be extended but we don't know by how much so
		// we depend on append after all data with known sizes fit.
		// We stop at that because we deal with a lot of columns here
		// which makes the required allocation size hard to guess.
		data := conn.writeBuffer.Make(maskLen + typesLen)
		nullMask = data[:maskLen]
		// No need to clean nullMask as make ensures that.
		pos := maskLen

		for i := range nullMask {
			nullMask[i] = 0
		}

		// newParameterBoundFlag 1 [1 byte]
		data[pos] = 0x01
		pos++

		// type of each parameter [len(args)*2 bytes]
		paramTypes := data[pos:]

		// value of each parameter [n bytes]

		for i, arg := range args {
			// build NULL-bitmap
			if arg == nil {
				nullMask[i/8] |= 1 << (uint(i) & 7)
				paramTypes[i+i] = byte(fieldTypeNULL)
				paramTypes[i+i+1] = 0x00
				continue
			}

			// cache types and values
			switch v := arg.(type) {
			case int8, int16, int32:
				paramTypes[i+i] = byte(fieldTypeLong)
				paramTypes[i+i+1] = 0x00

				b := argbuf.Make(4)
				uint32ToBytes(touint32(v), b)
			case int, int64:
				paramTypes[i+i] = byte(fieldTypeLongLong)
				paramTypes[i+i+1] = 0x00

				b := argbuf.Make(8)
				uint64ToBytes(touint64(v), b)
			case uint8, uint16, uint32:
				paramTypes[i+i] = byte(fieldTypeLongLong)
				paramTypes[i+i+1] = 0x80

				b := argbuf.Make(4)
				uint32ToBytes(touint32(v), b)
			case uint, uint64:
				paramTypes[i+i] = byte(fieldTypeLongLong)
				paramTypes[i+i+1] = 0x80 // type is unsigned
				b := argbuf.Make(8)
				uint64ToBytes(touint64(v), b)
			case float64:
				paramTypes[i+i] = byte(fieldTypeDouble)
				paramTypes[i+i+1] = 0x00

				b := argbuf.Make(8)
				uint64ToBytes(math.Float64bits(v), b)
			case float32:
				paramTypes[i+i] = byte(fieldTypeFloat)
				paramTypes[i+i+1] = 0x00
				b := argbuf.Make(4)
				uint32ToBytes(math.Float32bits(v), b)
			case bool:
				paramTypes[i+i] = byte(fieldTypeTiny)
				paramTypes[i+i+1] = 0x00
				b := argbuf.Make(1)
				if v {
					b[0] = 0x01
				} else {
					b[0] = 0x00
				}

			case []byte:
				// Common case (non-nil value) first
				if v != nil {
					paramTypes[i+i] = byte(fieldTypeString)
					paramTypes[i+i+1] = 0x00

					if len(v) < max_packet_size/(stmt.numInput+1) {
						Writelenmsg(argbuf, v)

					} else {
						return errors.New("输入的[]byte数据超过设计数值，请联系作者完善")

					}
					continue
				}

				// Handle []byte(nil) as a NULL value
				nullMask[i/8] |= 1 << (uint(i) & 7)
				paramTypes[i+i] = byte(fieldTypeNULL)
				paramTypes[i+i+1] = 0x00

			case string:
				paramTypes[i+i] = byte(fieldTypeString)
				paramTypes[i+i+1] = 0x00

				if len(v) < max_packet_size/(stmt.numInput+1) {
					Writelenmsg(argbuf, Str2bytes(v))
				} else {
					return errors.New("输入的[]byte数据超过设计数值，请联系作者完善")
				}

			case time.Time:
				paramTypes[i+i] = byte(fieldTypeString)
				paramTypes[i+i+1] = 0x00

				if v.IsZero() {
					Writelenmsg(argbuf, Str2bytes("0000-00-00"))
				} else {
					Writelenmsg(argbuf, Str2bytes(v.In(conn.loc).Format("2006-01-02 15:04:05.999999")))
				}

			default:
				r := reflect.TypeOf(arg)
				for r.Kind() == reflect.Ptr {
					r = r.Elem()
				}
				if r.Kind() == reflect.Struct || r.Kind() == reflect.Slice || r.Kind() == reflect.Map {
					paramTypes[i+i] = byte(fieldTypeString)
					paramTypes[i+i+1] = 0x00
					v := JsonMarshal(arg)
					if len(v) < max_packet_size/(stmt.numInput+1) {
						Writelenmsg(argbuf, v)
						continue
					} else {
						return errors.New("输入的[]byte数据超过设计数值，请联系作者完善")
					}
				}
				return fmt.Errorf("cannot convert type: %T", arg)
			}
		}

		// Check if param values exceeded the available buffer
		// In that case we must build the data packet with the new values buffer
	}
	conn.writeBuffer.Write(argbuf.Bytes())
	msglen := conn.writeBuffer.Len() - 4
	data := conn.writeBuffer.Bytes()
	data[0] = byte(msglen)
	data[1] = byte(msglen >> 8)
	data[2] = byte(msglen >> 16)
	data[3] = 0
	data[4] = 23 //StmtExecute
	data[5] = byte(stmt.id)
	data[6] = byte(stmt.id >> 8)
	data[7] = byte(stmt.id >> 16)
	data[8] = byte(stmt.id >> 24)
	// flags (0: CURSOR_TYPE_NO_CURSOR) [1 byte]
	data[9] = 0x00

	// iteration_count (uint32(1)) [4 bytes]
	data[10] = 0x01
	data[11] = 0x00
	data[12] = 0x00
	data[13] = 0x00
	n, err := conn.conn.Write(data)
	if n != msglen+4 {
		DEBUG("长度错误")
	}
	return err
}
func (stmt Database_mysql_stmt) NumInput() int {
	return stmt.numInput
}
func (stmt *Database_mysql_stmt) Close() (err error) {
	if stmt.numInput == -1 {
		return
	}
	stmt.conn.stmtMutex.Lock()
	defer stmt.conn.stmtMutex.Unlock()

	buf := stmtbufpool.Get().(*MsgBuffer)
	defer stmtbufpool.Put(buf)

	buf.Reset()
	data := buf.Make(9)
	data[0] = 5
	data[1] = 0
	data[2] = 0
	data[3] = 0
	data[4] = 25
	data[5] = byte(stmt.id)
	data[6] = byte(stmt.id >> 8)
	data[7] = byte(stmt.id >> 16)
	data[8] = byte(stmt.id >> 24)
	_, err = stmt.conn.conn.Write(data)
	if err != nil {
		return err
	}
	delete(stmt.conn.stmtCache, stmt.query)

	return err
}
func (stmt Database_mysql_stmt) LastInsertId() (int64, error) {
	return stmt.lastInsertId, nil
}

func (stmt Database_mysql_stmt) RowsAffected() (int64, error) {
	return stmt.rowsAffected, nil
}
