package com.lsyl.tunnel.mobile.tunnel

import com.lsyl.tunnel.mobile.profile.DIRECTION_CLIENT_TO_SERVER
import com.lsyl.tunnel.mobile.profile.ForwardConfig
import com.lsyl.tunnel.mobile.protocol.OpenResponse
import com.lsyl.tunnel.mobile.protocol.TunnelProtocol
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.ServerSocket
import java.net.Socket
import java.net.SocketTimeoutException
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger

class LocalForwardResilienceTest {
    @Test
    fun streamTimeoutKeepsListenerAvailableForNextConnection() {
        val port = freeLoopbackPort()
        val forward = ForwardConfig("rdp", DIRECTION_CLIENT_TO_SERVER, "127.0.0.1:$port", "192.168.1.7:3389")
        val runtime = ForwardRuntime(forward)
        val protocol = TimeoutProtocol(expectedAttempts = 2)
        val listeners = Executors.newSingleThreadExecutor()
        val connections = Executors.newFixedThreadPool(2)
        val copies = Executors.newFixedThreadPool(2)
        val local = LocalForward(forward, protocol, runtime, listeners, connections, copies)

        try {
            local.start()
            Socket().use { it.connect(InetSocketAddress("127.0.0.1", port), 2_000) }
            Socket().use { it.connect(InetSocketAddress("127.0.0.1", port), 2_000) }

            assertTrue("two local attempts were not accepted", protocol.awaitAttempts())
            assertTrue("stream failure was not published", waitForIssue(runtime, "network_timeout"))
            val status = runtime.snapshot()
            assertTrue(local.isRunning())
            assertEquals(ForwardState.LISTENING, status.state)
            assertEquals("network_timeout", status.issue?.code)
            assertEquals(2, protocol.attempts.get())
        } finally {
            local.stop()
            listeners.shutdownNow()
            connections.shutdownNow()
            copies.shutdownNow()
        }
    }

    @Test
    fun successfulStreamClearsPreviousTransientIssue() {
        val port = freeLoopbackPort()
        val forward = ForwardConfig("rdp", DIRECTION_CLIENT_TO_SERVER, "127.0.0.1:$port", "192.168.1.7:3389")
        val runtime = ForwardRuntime(forward)
        val protocol = RecoveringProtocol()
        val listeners = Executors.newSingleThreadExecutor()
        val connections = Executors.newSingleThreadExecutor()
        val copies = Executors.newFixedThreadPool(2)
        val local = LocalForward(forward, protocol, runtime, listeners, connections, copies)

        try {
            local.start()
            Socket().use { it.connect(InetSocketAddress("127.0.0.1", port), 2_000) }
            assertTrue("first failure was not published", waitForIssue(runtime, "network_timeout"))

            Socket().use { it.connect(InetSocketAddress("127.0.0.1", port), 2_000) }
            assertTrue("second stream did not open", protocol.awaitSuccess())
            assertTrue("old issue remained after recovery", waitFor { runtime.snapshot().issue == null })
            assertEquals(ForwardState.LISTENING, runtime.snapshot().state)
        } finally {
            local.stop()
            listeners.shutdownNow()
            connections.shutdownNow()
            copies.shutdownNow()
        }
    }

    private fun freeLoopbackPort(): Int =
        ServerSocket(0, 1, InetAddress.getByName("127.0.0.1")).use { it.localPort }

    private fun waitForIssue(runtime: ForwardRuntime, code: String): Boolean {
        return waitFor { runtime.snapshot().issue?.code == code }
    }

    private fun waitFor(condition: () -> Boolean): Boolean {
        val deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(5)
        while (System.nanoTime() < deadline) {
            if (condition()) return true
            Thread.sleep(10)
        }
        return false
    }

    private class TimeoutProtocol(expectedAttempts: Int) : TunnelProtocol {
        val attempts = AtomicInteger(0)
        private val attemptLatch = CountDownLatch(expectedAttempts)

        override fun health(): OpenResponse = OpenResponse(true, "", "")

        override fun forwardCheck(forward: ForwardConfig): OpenResponse = OpenResponse(true, "", "")

        override fun open(forward: ForwardConfig): Socket {
            attempts.incrementAndGet()
            attemptLatch.countDown()
            throw SocketTimeoutException("timed out")
        }

        fun awaitAttempts(): Boolean = attemptLatch.await(5, TimeUnit.SECONDS)
    }

    private class RecoveringProtocol : TunnelProtocol {
        private val attempts = AtomicInteger(0)
        private val successLatch = CountDownLatch(1)

        override fun health(): OpenResponse = OpenResponse(true, "", "")

        override fun forwardCheck(forward: ForwardConfig): OpenResponse = OpenResponse(true, "", "")

        override fun open(forward: ForwardConfig): Socket {
            if (attempts.incrementAndGet() == 1) throw SocketTimeoutException("timed out")
            successLatch.countDown()
            return Socket()
        }

        fun awaitSuccess(): Boolean = successLatch.await(5, TimeUnit.SECONDS)
    }
}
