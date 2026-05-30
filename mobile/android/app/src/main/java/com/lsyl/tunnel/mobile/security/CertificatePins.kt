package com.lsyl.tunnel.mobile.security

import java.io.ByteArrayInputStream
import java.security.MessageDigest
import java.security.cert.CertificateFactory
import java.security.cert.X509Certificate

object CertificatePins {
    fun parseCertificate(bytes: ByteArray): X509Certificate {
        val factory = CertificateFactory.getInstance("X.509")
        return factory.generateCertificate(ByteArrayInputStream(bytes)) as X509Certificate
    }

    fun sha256Hex(cert: X509Certificate): String = sha256Hex(cert.encoded)

    fun sha256Hex(bytes: ByteArray): String {
        val digest = MessageDigest.getInstance("SHA-256").digest(bytes)
        return digest.joinToString("") { "%02x".format(it) }
    }
}
