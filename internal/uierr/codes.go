package uierr

// 用户会读到的每一种失败。
//
// 命名是 <领域>.<发生了什么>，而不是按异常类型分——分类的依据是
// 「他的下一步不同」：配对码过期要重新生成一个，配对码用过了要换一个，
// 这两件事在界面上是两句不同的话。
const (
	// 配对：客户端扫码或手输配对码时
	PairNotFound = "pair.not_found"
	PairUsed     = "pair.used"
	PairExpired  = "pair.expired"

	// 会话：客户端的设备凭据
	SessionInvalid = "session.invalid"

	// 附件：浏览器或通知里打开一个签名链接时
	FileLinkInvalid = "file.link_invalid"
	FileGone        = "file.gone"

	// 发送：频率限制。发送方与接入页都会撞上
	SendTooFast  = "send.too_fast"
	JoinTooFast  = "join.too_fast"
	PairTooFast  = "pair.too_fast"
	LoginTooFast = "login.too_fast"

	// 回复：客户端提交一个不在范围里的数值
	ReplyOutOfRange = "reply.out_of_range"
)

// All 全部 code。测试拿它核对语料里一条不少——
// 少一条的表现是用户看到一个 "pair.expired" 这样的字符串，而不是一句话。
var All = []string{
	PairNotFound, PairUsed, PairExpired,
	SessionInvalid,
	FileLinkInvalid, FileGone,
	SendTooFast, JoinTooFast, PairTooFast, LoginTooFast,
	ReplyOutOfRange,
}
