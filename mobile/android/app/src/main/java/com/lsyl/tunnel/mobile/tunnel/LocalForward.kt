package com.lsyl.tunnel.mobile.tunnel

import com.lsyl.tunnel.mobile.profile.ForwardConfig
import com.lsyl.tunnel.mobile.protocol.ProtocolClient
import com.lsyl.tunnel.mobile.protocol.ProtocolException
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.ServerSocket
import java.net.Socket
import java.net.SocketException
import java.util.concurrent.Executor
import java.util.concurrent.RejectedExecutionException
import java.util.concurrent.atomic.AtomicBoolean

class LocalForward(
    private val forward: ForwardConfig,
    private val protocol: ProtocolClient,
    private val runtime: ForwardRuntime,
    private val listenerExecutor: Executor,
    private val connectionExecutor: Executor,
    private val copyExecutor: Executor
) {
    private val running = AtomicBoolean(false)
    @Volatile private var serverSocket: ServerSocket? = null

    fun start() {
        if (!running.compareAndSet(false, true)) return
        runtime.setState(ForwardState.STARTING, "正在监听本地端口")
        try {
            val endpoint = forward.localEndpoint()
            val socket = ServerSocket()
            socket.reuseAddress = true
            socket.bind(InetSocketAddress(InetAddress.getByName("127.0.0.1"), endpoint.port))
            serverSocket = socket
            runtime.setState(ForwardState.LISTENING, "本地端口监听中")
            listenerExecutor.execute { acceptLoop(socket) }
        } catch (err: Exception) {
            running.set(false)
            try {
                serverSocket?.close()
            } catch (_: Exception) {
            }
            serverSocket = null
            runtime.setState(ForwardState.LISTEN_FAILED, friendlyMessage(err))
        }
    }

    fun stop(message: String = "已停止", state: ForwardState = ForwardState.STOPPED) {
        running.set(false)
        try {
            serverSocket?.close()
        } catch (_: Exception) {
        } finally {
            serverSocket = null
            runtime.setState(state, message)
        }
    }

    fun isRunning(): Boolean = running.get()

    private fun acceptLoop(listener: ServerSocket) {
        while (running.get()) {
            try {
                val local = listener.accept()
                try {
                    connectionExecutor.execute { handleLocal(local) }
                } catch (_: RejectedExecutionException) {
                    closeQuietly(local)
                    runtime.setState(ForwardState.LISTENING, "本地连接并发已达上限，已拒绝")
                }
            } catch (_: SocketException) {
                if (running.get()) runtime.setState(ForwardState.ERROR, "本地监听已中断")
                return
            } catch (err: Exception) {
                runtime.setState(ForwardState.ERROR, friendlyMessage(err))
            }
        }
    }

    private fun handleLocal(local: Socket) {
        val streamDone = runtime.beginStream()
        try {
            val remote = protocol.open(forward)
            SocketPipe.copyBidirectional(local, remote, copyExecutor)
        } catch (err: ProtocolException) {
            closeQuietly(local)
            if (err.response.code == "target_denied") {
                runtime.setState(ForwardState.REJECTED, "当前账号没有访问该端口的权限")
                stop("当前账号没有访问该端口的权限", ForwardState.REJECTED)
            } else {
                runtime.setState(ForwardState.ERROR, err.message ?: "连接异常")
            }
        } catch (err: Exception) {
            closeQuietly(local)
            runtime.setState(ForwardState.ERROR, friendlyMessage(err))
        } finally {
            streamDone()
        }
    }

    private fun closeQuietly(socket: Socket) {
        try {
            socket.close()
        } catch (_: Exception) {
        }
    }

    private fun friendlyMessage(err: Exception): String = err.message?.takeIf { it.isNotBlank() } ?: "连接异常"
}
