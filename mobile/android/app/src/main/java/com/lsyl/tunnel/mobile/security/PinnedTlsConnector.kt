package com.lsyl.tunnel.mobile.security

import com.lsyl.tunnel.mobile.profile.MobileProfile
import com.lsyl.tunnel.mobile.profile.HostPort
import com.lsyl.tunnel.mobile.protocol.Ipv6EndpointPool
import java.io.Closeable
import java.net.InetSocketAddress
import java.net.Socket
import java.security.KeyStore
import java.security.cert.X509Certificate
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicBoolean
import javax.net.ssl.SSLContext
import javax.net.ssl.SSLParameters
import javax.net.ssl.SSLSocket
import javax.net.ssl.TrustManagerFactory

internal class PinnedTlsConnector(private val certBytes: ByteArray) {
    private val certificate = CertificatePins.parseCertificate(certBytes)
    private val expectedPin = CertificatePins.sha256Hex(certificate)
    private val context: SSLContext = buildContext()
    private val pending = PendingCloseables<Socket>()

    fun connect(profile: MobileProfile, pool: Ipv6EndpointPool? = null): SSLSocket {
        val endpoint = profile.serverEndpoint()
        val timeout = profile.connection.timeoutMillis()
        val raw = if (pool?.hasCandidates == true) pool.dial(timeout, ::dialRaw) else dialRaw(endpoint, timeout)
        return try {
            raw.closeOnFailure {
                raw.soTimeout = timeout
                CertificateGate.perform(raw, certificate)
                // The alternate IPv6 address changes transport only, not server identity.
                val tlsHost = profile.tls.serverName.ifBlank { endpoint.host }
                val tls = pending.track(
                    context.socketFactory.createSocket(raw, tlsHost, endpoint.port, true) as SSLSocket
                )
                pending.release(raw)
                try {
                    tls.closeOnFailure { socket ->
                        socket.soTimeout = timeout
                        if (!socket.supportedProtocols.contains(REQUIRED_TLS)) {
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
                        socket
                    }
                } catch (failure: Throwable) {
                    pending.release(tls)
                    throw failure
                }
            }
        } catch (failure: Throwable) {
            pending.release(raw)
            throw failure
        }
    }

    fun release(socket: Socket) {
        pending.release(socket)
    }

    fun cancelPending() {
        pending.cancelAll()
    }

    private fun dialRaw(endpoint: HostPort, timeout: Int): Socket {
        val raw = pending.track(Socket())
        return try {
            raw.closeOnFailure {
                raw.connect(InetSocketAddress(endpoint.host, endpoint.port), timeout)
                raw
            }
        } catch (failure: Throwable) {
            pending.release(raw)
            throw failure
        }
    }

    private fun buildContext(): SSLContext {
        val keyStore = KeyStore.getInstance(KeyStore.getDefaultType())
        keyStore.load(null)
        keyStore.setCertificateEntry("lsyl-server", certificate)
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

internal inline fun <T : Closeable, R> T.closeOnFailure(block: (T) -> R): R {
    try {
        return block(this)
    } catch (failure: Throwable) {
        try {
            close()
        } catch (closeFailure: Throwable) {
            failure.addSuppressed(closeFailure)
        }
        throw failure
    }
}

internal class PendingCloseables<T : Closeable> {
    private val cancelled = AtomicBoolean(false)
    private val items = ConcurrentHashMap.newKeySet<T>()

    fun <R : T> track(value: R): R {
        if (cancelled.get()) {
            value.closeQuietly()
            return value
        }
        items.add(value)
        if (cancelled.get() && items.remove(value)) value.closeQuietly()
        return value
    }

    fun release(value: T) {
        items.remove(value)
    }

    fun cancelAll() {
        cancelled.set(true)
        items.toList().forEach { value ->
            if (items.remove(value)) value.closeQuietly()
        }
    }

    private fun Closeable.closeQuietly() {
        try {
            close()
        } catch (_: Exception) {
        }
    }
}
