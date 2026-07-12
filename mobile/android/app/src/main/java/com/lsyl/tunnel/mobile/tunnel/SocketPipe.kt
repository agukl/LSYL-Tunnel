package com.lsyl.tunnel.mobile.tunnel

import java.net.Socket
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executor
import java.util.concurrent.atomic.AtomicBoolean

object SocketPipe {
    fun copyBidirectional(left: Socket, right: Socket, executor: Executor) {
        val done = CountDownLatch(1)
        val closed = AtomicBoolean(false)
        val finish = {
            if (closed.compareAndSet(false, true)) {
                closeQuietly(left)
                closeQuietly(right)
                done.countDown()
            }
        }
        try {
            executor.execute { copyOneWay(left, right, finish) }
            executor.execute { copyOneWay(right, left, finish) }
            done.await()
        } finally {
            finish()
        }
    }

    private fun copyOneWay(source: Socket, destination: Socket, finish: () -> Unit) {
        try {
            val input = source.getInputStream()
            val output = destination.getOutputStream()
            val buffer = ByteArray(16 * 1024)
            while (true) {
                val n = input.read(buffer)
                if (n < 0) break
                output.write(buffer, 0, n)
                output.flush()
            }
        } catch (_: Exception) {
        } finally {
            finish()
        }
    }

    private fun closeQuietly(socket: Socket) {
        try {
            socket.close()
        } catch (_: Exception) {
        }
    }
}
