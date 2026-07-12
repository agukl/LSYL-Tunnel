package com.lsyl.tunnel.mobile.tunnel

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.InputStream
import java.io.OutputStream
import java.net.Socket
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit

class SocketPipeTest {
    @Test
    fun closingOneDirectionClosesBothSocketsWithoutWaitingForHalfClose() {
        val left = TestSocket(ByteArrayInputStream("request".toByteArray()), ByteArrayOutputStream())
        val blockedInput = ClosingInputStream()
        val rightOutput = ByteArrayOutputStream()
        val right = TestSocket(blockedInput, rightOutput)
        val copyExecutor = Executors.newFixedThreadPool(2)
        val caller = Executors.newSingleThreadExecutor()
        try {
            caller.submit { SocketPipe.copyBidirectional(left, right, copyExecutor) }.get(2, TimeUnit.SECONDS)

            assertArrayEquals("request".toByteArray(), rightOutput.toByteArray())
            assertTrue(left.closed)
            assertTrue(right.closed)
        } finally {
            caller.shutdownNow()
            copyExecutor.shutdownNow()
        }
    }

    private class TestSocket(
        private val input: InputStream,
        private val output: OutputStream
    ) : Socket() {
        @Volatile var closed = false

        override fun getInputStream(): InputStream = input

        override fun getOutputStream(): OutputStream = output

        override fun close() {
            closed = true
            input.close()
            output.close()
        }
    }

    private class ClosingInputStream : InputStream() {
        private val closed = CountDownLatch(1)

        override fun read(): Int {
            closed.await()
            return -1
        }

        override fun close() {
            closed.countDown()
        }
    }
}
