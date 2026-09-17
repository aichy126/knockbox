// Package dao 数据访问层。只做 SQL，不含业务判断。
package dao

import (
	"fmt"

	"github.com/aichy126/knockbox/internal/models"
	"xorm.io/xorm"
)

type DAO struct {
	e *xorm.Engine
}

func New(engine *xorm.Engine) *DAO { return &DAO{e: engine} }

// Engine 暴露给需要裸 SQL 的场景（清空、GC、统计）。
func (d *DAO) Engine() *xorm.Engine { return d.e }

// Tx 在一个事务里执行 fn。
//
// 写事务以 BEGIN IMMEDIATE 开始——这由 DSN 上的 _txlock=immediate 保证，不是这里做的。
// 默认的 deferred 会在「先读后写、中途有别的写者提交」时报 SQLITE_BUSY，
// 且 busy_timeout 对这种锁升级失败不生效（internal/dao/txlock_test.go 复现了这个差异）。
func (d *DAO) Tx(fn func(sess *xorm.Session) error) error {
	sess := d.e.NewSession()
	defer func() { _ = sess.Close() }()
	if err := sess.Begin(); err != nil {
		return err
	}
	if err := fn(sess); err != nil {
		_ = sess.Rollback()
		return err
	}
	return sess.Commit()
}

// NextRev 取下一个全局修订号。
//
// 必须在调用方的事务里执行：rev 的分配和「用这个 rev 写的那一行」要么一起成功、要么一起回滚，
// 否则会在同步流里留下一个没有对应数据的空洞，客户端的游标越过它就再也拉不回来了。
func NextRev(sess *xorm.Session) (int64, error) {
	if _, err := sess.Exec("UPDATE seq SET val = val + 1 WHERE name = 'rev'"); err != nil {
		return 0, err
	}
	rows, err := sess.QueryString("SELECT val FROM seq WHERE name = 'rev'")
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("seq 表里没有 rev 行，数据库未正确初始化")
	}
	var rev int64
	if _, err := fmt.Sscan(rows[0]["val"], &rev); err != nil {
		return 0, fmt.Errorf("rev 值 %q 非法: %w", rows[0]["val"], err)
	}
	return rev, nil
}

// CurrentRev 读当前最大 rev，不递增。用于同步响应里的水位线。
func (d *DAO) CurrentRev() (int64, error) {
	rows, err := d.e.QueryString("SELECT val FROM seq WHERE name = 'rev'")
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	var rev int64
	_, err = fmt.Sscan(rows[0]["val"], &rev)
	return rev, err
}

// --- kv -------------------------------------------------------------------

func (d *DAO) KVGet(key string) (string, bool, error) {
	var kv models.KV
	has, err := d.e.Where("k = ?", key).Get(&kv)
	return kv.V, has, err
}

func (d *DAO) KVSet(key, val string, now int64) error {
	affected, err := d.e.Exec("UPDATE kv SET v = ?, updated_at = ? WHERE k = ?", val, now, key)
	if err != nil {
		return err
	}
	if n, _ := affected.RowsAffected(); n > 0 {
		return nil
	}
	_, err = d.e.Exec("INSERT INTO kv(k, v, updated_at) VALUES (?, ?, ?)", key, val, now)
	return err
}
