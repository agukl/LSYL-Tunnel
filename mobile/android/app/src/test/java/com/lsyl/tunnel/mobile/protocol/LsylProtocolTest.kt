package com.lsyl.tunnel.mobile.protocol

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.fail
import org.junit.Test
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.nio.ByteBuffer

class LsylProtocolTest {
    @Test
    fun writesAndReadsLengthPrefixedJson() {
        val output = ByteArrayOutputStream()
        LsylProtocol.writeJson(output, JSONObject().put("type", "health"))

        val decoded = LsylProtocol.readJson(ByteArrayInputStream(output.toByteArray()))

        assertEquals("health", decoded.getString("type"))
    }

    @Test
    fun rejectsInvalidHandshakeLength() {
        val input = ByteArrayInputStream(ByteBuffer.allocate(4).putInt(MAX_HANDSHAKE_BYTES + 1).array())

        try {
            LsylProtocol.readJson(input)
            fail("expected invalid handshake length to be rejected")
        } catch (err: IllegalArgumentException) {
            assertEquals("handshake too large: ${MAX_HANDSHAKE_BYTES + 1}", err.message)
        }
    }
}
