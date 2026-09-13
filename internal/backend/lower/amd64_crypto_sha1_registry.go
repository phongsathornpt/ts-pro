package lower

import "github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"

func emitAMD64SHA1RuntimeSymbols(e *amd64.Emitter, fnOffsets map[string]int) {
	fnOffsets["ts_crypto_sha1_compress"] = len(e.Code)
	emitAMD64SHA1Compress(e)
	fnOffsets["ts_crypto_sha1"] = len(e.Code)
	emitAMD64SHA1(e, fnOffsets["ts_crypto_sha1_compress"], fnOffsets["ts_byte_buffer_new"])
}
