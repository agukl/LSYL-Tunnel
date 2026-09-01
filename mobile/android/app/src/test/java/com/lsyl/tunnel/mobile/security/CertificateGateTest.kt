package com.lsyl.tunnel.mobile.security

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Test

class CertificateGateTest {
    @Test
    fun responseMatchesGoWireFormat() {
        val certificateDer = ByteArray(64) { it.toByte() }
        val challenge = ByteArray(41)
        "LSYLGATE".toByteArray(Charsets.US_ASCII).copyInto(challenge)
        challenge[8] = 1
        repeat(32) { challenge[9 + it] = it.toByte() }
        val clientNonce = ByteArray(32) { (it + 32).toByte() }

        val response = CertificateGate.buildResponse(certificateDer, challenge, clientNonce)

        assertEquals(73, response.size)
        assertArrayEquals("LSYLPROF".toByteArray(Charsets.US_ASCII), response.copyOfRange(0, 8))
        assertEquals(1, response[8].toInt())
        assertArrayEquals(clientNonce, response.copyOfRange(9, 41))
        assertArrayEquals(
            hex("174c4c8b670c9fb3c6c236000e363debe79023bb7d7001d6cd1c9aad8715f329"),
            response.copyOfRange(41, 73)
        )
    }

    private fun hex(value: String): ByteArray = ByteArray(value.length / 2) { index ->
        value.substring(index * 2, index * 2 + 2).toInt(16).toByte()
    }
}
