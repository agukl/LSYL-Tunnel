package com.lsyl.tunnel.mobile.tunnel

import com.lsyl.tunnel.mobile.profile.ConnectionConfig
import com.lsyl.tunnel.mobile.profile.DIRECTION_CLIENT_TO_SERVER
import com.lsyl.tunnel.mobile.profile.ForwardConfig
import com.lsyl.tunnel.mobile.profile.LoadedProfile
import com.lsyl.tunnel.mobile.profile.MobileProfile
import com.lsyl.tunnel.mobile.profile.SavedCredential
import com.lsyl.tunnel.mobile.profile.TlsConfig
import com.lsyl.tunnel.mobile.protocol.OpenResponse
import com.lsyl.tunnel.mobile.protocol.ProtocolException
import com.lsyl.tunnel.mobile.protocol.TunnelProtocol
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.net.InetAddress
import java.net.ServerSocket
import java.net.Socket
import java.net.SocketTimeoutException

class TunnelManagerTest {
    @Test
    fun remoteTimeoutDoesNotCloseStartedListeners() {
        val forward = forward("rdp", freeLoopbackPort(), "192.168.1.7:3389")
        val protocol = ControllableProtocol(healthFailure = SocketTimeoutException("timed out"))
        val manager = TunnelManager(loadedProfile(listOf(forward)), protocol)

        try {
            manager.startListeners()
            try {
                manager.checkRemote()
            } catch (_: SocketTimeoutException) {
            }

            val status = manager.stats().forwards.single()
            assertTrue(manager.stats().running)
            assertEquals(ForwardState.LISTENING, status.state)
        } finally {
            manager.stop()
        }
    }

    @Test
    fun deniedRuleDoesNotStopAnotherRuleAndCanRecoverLater() {
        val denied = forward("denied", freeLoopbackPort(), "192.168.1.7:3389")
        val allowed = forward("allowed", freeLoopbackPort(), "192.168.1.8:3389")
        val protocol = ControllableProtocol(deniedRule = "denied")
        val manager = TunnelManager(loadedProfile(listOf(denied, allowed)), protocol)

        try {
            manager.startListeners()
            manager.checkRemote()

            var statuses = manager.stats().forwards.associateBy { it.name }
            assertEquals(ForwardState.REJECTED, statuses.getValue("denied").state)
            assertEquals("target_denied", statuses.getValue("denied").issue?.code)
            assertEquals(ForwardState.LISTENING, statuses.getValue("allowed").state)

            protocol.deniedRule = null
            manager.checkRemote()

            statuses = manager.stats().forwards.associateBy { it.name }
            assertEquals(ForwardState.LISTENING, statuses.getValue("denied").state)
            assertEquals(null, statuses.getValue("denied").issue)
            assertEquals(ForwardState.LISTENING, statuses.getValue("allowed").state)
        } finally {
            manager.stop()
        }
    }

    private fun forward(name: String, port: Int, target: String): ForwardConfig =
        ForwardConfig(name, DIRECTION_CLIENT_TO_SERVER, "127.0.0.1:$port", target)

    private fun loadedProfile(forwards: List<ForwardConfig>): LoadedProfile = LoadedProfile(
        profile = MobileProfile(
            version = 1,
            profileId = "test",
            serverAddr = "example.test:3443",
            username = "alice",
            clientId = "android-test",
            savedCredential = SavedCredential("server_sealed", "key", "2099-01-01T00:00:00Z", "sealed"),
            tls = TlsConfig("server.crt", "", "1.3", false),
            connection = ConnectionConfig(5),
            forwards = forwards
        ),
        serverCertBytes = byteArrayOf()
    )

    private fun freeLoopbackPort(): Int =
        ServerSocket(0, 1, InetAddress.getByName("127.0.0.1")).use { it.localPort }

    private class ControllableProtocol(
        private var healthFailure: Throwable? = null,
        var deniedRule: String? = null
    ) : TunnelProtocol {
        override fun health(): OpenResponse {
            healthFailure?.let { throw it }
            return OpenResponse(true, "", "")
        }

        override fun forwardCheck(forward: ForwardConfig): OpenResponse {
            if (forward.name == deniedRule) {
                throw ProtocolException(OpenResponse(false, "target_denied", "not allowed"))
            }
            return OpenResponse(true, "", "")
        }

        override fun open(forward: ForwardConfig): Socket = error("not used")
    }
}
