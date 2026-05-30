package com.lsyl.tunnel.mobile.tunnel

import java.io.InputStream
import java.io.OutputStream
import java.net.Socket
import java.util.concurrent.CountDownLatch
import java.util.concurrent.ExecutorService

object SocketPipe {
    fun copyBidirectional(left: Socket, right: Socket, executor: ExecutorService) {
        val done = CountDownLatch(2)
        executor.execute { copyOneWay(left.getInputStream(), right.getOutputStream(), done) }
        executor.execute { copyOneWay(right.getInputStream(), left.getOutputStream(), done) }
        try {
            done.await()
        } finally {
            closeQuietly(left)
            closeQuietly(right)
        }
    }

    private fun copyOneWay(input: InputStream, output: OutputStream, done: CountDownLatch) {
        try {
            val buffer = ByteArray(16 * 1024)
            while (true) {
                val n = input.read(buffer)
                if (n < 0) break
                output.write(buffer, 0, n)
                output.flush()
            }
        } catch (_: Exception) {
        } finally {
            done.countDown()
        }
    }

    private fun closeQuietly(socket: Socket) {
        try {
            socket.close()
        } catch (_: Exception) {
        }
    }
}
