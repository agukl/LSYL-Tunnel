package com.lsyl.tunnel.mobile.security

import java.io.EOFException
import java.io.IOException
import java.net.Socket
import java.security.MessageDigest
import java.security.SecureRandom
import java.security.cert.X509Certificate
import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec

internal object CertificateGate {
    private const val VERSION: Byte = 1
    private const val NONCE_SIZE = 32
    private const val HASH_SIZE = 32
    private val challengeMagic = "LSYLGATE".toByteArray(Charsets.US_ASCII)
    private val responseMagic = "LSYLPROF".toByteArray(Charsets.US_ASCII)
    private val keyContext = "LSYL-TUNNEL-CERTIFICATE-GATE-KEY-V1\u0000".toByteArray(Charsets.UTF_8)
    private val proofContext = "LSYL-TUNNEL-CERTIFICATE-GATE-PROOF-V1\u0000".toByteArray(Charsets.UTF_8)
    private val random = SecureRandom()

    private const val CHALLENGE_SIZE = 8 + 1 + NONCE_SIZE
    private const val RESPONSE_HEADER_SIZE = 8 + 1 + NONCE_SIZE
    private const val RESPONSE_SIZE = RESPONSE_HEADER_SIZE + HASH_SIZE

    fun perform(socket: Socket, certificate: X509Certificate) {
        val challenge = ByteArray(CHALLENGE_SIZE)
        readFully(socket, challenge)
        if (!challenge.copyOfRange(0, 8).contentEquals(challengeMagic) || challenge[8] != VERSION) {
            throw IOException("服务端不支持当前证书持有前置门协议")
        }

        val clientNonce = ByteArray(NONCE_SIZE).also(random::nextBytes)
        val response = buildResponse(certificate.encoded, challenge, clientNonce)
        socket.getOutputStream().apply {
            write(response)
            flush()
        }
    }

    internal fun buildResponse(certificateDer: ByteArray, challenge: ByteArray, clientNonce: ByteArray): ByteArray {
        require(challenge.size == CHALLENGE_SIZE) { "invalid certificate gate challenge size" }
        require(clientNonce.size == NONCE_SIZE) { "invalid certificate gate client nonce size" }
        val response = ByteArray(RESPONSE_SIZE)
        responseMagic.copyInto(response)
        response[8] = VERSION
        clientNonce.copyInto(response, 9)

        val keyDigest = MessageDigest.getInstance("SHA-256")
        keyDigest.update(keyContext)
        val gateKey = keyDigest.digest(certificateDer)
        val mac = Mac.getInstance("HmacSHA256")
        mac.init(SecretKeySpec(gateKey, "HmacSHA256"))
        mac.update(proofContext)
        mac.update(challenge)
        mac.update(response, 0, RESPONSE_HEADER_SIZE)
        mac.doFinal().copyInto(response, RESPONSE_HEADER_SIZE)
        return response
    }

    private fun readFully(socket: Socket, destination: ByteArray) {
        val input = socket.getInputStream()
        var offset = 0
        while (offset < destination.size) {
            val read = input.read(destination, offset, destination.size - offset)
            if (read < 0) throw EOFException("服务端在证书持有前置验证前断开连接")
            if (read == 0) continue
            offset += read
        }
    }
}
