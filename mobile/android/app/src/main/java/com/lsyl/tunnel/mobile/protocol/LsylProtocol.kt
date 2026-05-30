package com.lsyl.tunnel.mobile.protocol

import org.json.JSONObject
import java.io.EOFException
import java.io.InputStream
import java.io.OutputStream
import java.nio.ByteBuffer

const val MAX_HANDSHAKE_BYTES = 32 * 1024

object LsylProtocol {
    fun writeJson(out: OutputStream, json: JSONObject) {
        val body = json.toString().toByteArray(Charsets.UTF_8)
        require(body.isNotEmpty()) { "empty handshake" }
        require(body.size <= MAX_HANDSHAKE_BYTES) { "handshake too large: ${body.size}" }
        out.write(ByteBuffer.allocate(4).putInt(body.size).array())
        out.write(body)
        out.flush()
    }

    fun readJson(input: InputStream): JSONObject {
        val header = input.readExact(4)
        val n = ByteBuffer.wrap(header).int
        require(n > 0) { "empty handshake" }
        require(n <= MAX_HANDSHAKE_BYTES) { "handshake too large: $n" }
        return JSONObject(input.readExact(n).toString(Charsets.UTF_8))
    }

    private fun InputStream.readExact(size: Int): ByteArray {
        val out = ByteArray(size)
        var offset = 0
        while (offset < size) {
            val n = read(out, offset, size - offset)
            if (n < 0) throw EOFException("unexpected end of stream")
            offset += n
        }
        return out
    }
}
