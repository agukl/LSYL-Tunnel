package com.lsyl.tunnel.mobile.security

import com.lsyl.tunnel.mobile.profile.MobileProfile
import java.net.InetSocketAddress
import java.net.Socket
import java.security.KeyStore
import java.security.cert.X509Certificate
import javax.net.ssl.SSLContext
import javax.net.ssl.SSLParameters
import javax.net.ssl.SSLSocket
import javax.net.ssl.TrustManagerFactory

class PinnedTlsConnector(private val certBytes: ByteArray) {
    private val expectedPin = CertificatePins.sha256Hex(CertificatePins.parseCertificate(certBytes))
    private val context: SSLContext = buildContext()

    fun connect(profile: MobileProfile): SSLSocket {
        val endpoint = profile.serverEndpoint()
        val timeout = profile.connection.timeoutMillis()
        val raw = Socket()
        raw.connect(InetSocketAddress(endpoint.host, endpoint.port), timeout)
        val tlsHost = profile.tls.serverName.ifBlank { endpoint.host }
        val socket = context.socketFactory.createSocket(raw, tlsHost, endpoint.port, true) as SSLSocket
        socket.soTimeout = timeout
        if (!socket.supportedProtocols.contains(REQUIRED_TLS)) {
            socket.close()
            throw IllegalStateException("当前系统不支持 TLS 1.3，请使用 Android 10 及以上设备")
        }
        socket.enabledProtocols = arrayOf(REQUIRED_TLS)
        if (profile.tls.serverName.isNotBlank()) {
            val params: SSLParameters = socket.sslParameters
            params.endpointIdentificationAlgorithm = "HTTPS"
            socket.sslParameters = params
        }
        socket.startHandshake()
        verifyPeerPin(socket)
        return socket
    }

    private fun buildContext(): SSLContext {
        val cert = CertificatePins.parseCertificate(certBytes)
        val keyStore = KeyStore.getInstance(KeyStore.getDefaultType())
        keyStore.load(null)
        keyStore.setCertificateEntry("lsyl-server", cert)
        val tmf = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm())
        tmf.init(keyStore)
        return SSLContext.getInstance("TLS").apply {
            init(null, tmf.trustManagers, null)
        }
    }

    private fun verifyPeerPin(socket: SSLSocket) {
        val peer = socket.session.peerCertificates.firstOrNull() as? X509Certificate
            ?: throw SecurityException("server did not present an X509 certificate")
        val actual = CertificatePins.sha256Hex(peer)
        if (!actual.equals(expectedPin, ignoreCase = true)) {
            throw SecurityException("server certificate pin mismatch")
        }
    }

    companion object {
        private const val REQUIRED_TLS = "TLSv1.3"
    }
}
