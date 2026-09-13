package lower

import "github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"

func emitAMD64SHA2RuntimeSymbols(e *amd64.Emitter, fnOffsets map[string]int) {
	fnOffsets["ts_crypto_sha512_compress"] = len(e.Code)
	emitAMD64SHA512Compress(e)
	fnOffsets["ts_crypto_sha384"] = len(e.Code)
	emitAMD64SHA512Variant(e, fnOffsets["ts_crypto_sha512_compress"], fnOffsets["ts_byte_buffer_new"], amd64SHA384Initial, 48)
	fnOffsets["ts_crypto_hmac_sha384"] = len(e.Code)
	emitAMD64HMACSHA384(e, fnOffsets["ts_crypto_sha384"], fnOffsets["ts_byte_buffer_new"])
	fnOffsets["ts_crypto_sha512"] = len(e.Code)
	emitAMD64SHA512Variant(e, fnOffsets["ts_crypto_sha512_compress"], fnOffsets["ts_byte_buffer_new"], amd64SHA512Initial, 64)
	fnOffsets["ts_crypto_hmac_sha512"] = len(e.Code)
	emitAMD64HMACSHA512(e, fnOffsets["ts_crypto_sha512"], fnOffsets["ts_byte_buffer_new"])
}
