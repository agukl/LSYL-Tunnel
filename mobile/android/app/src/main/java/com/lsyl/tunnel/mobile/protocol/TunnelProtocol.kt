package com.lsyl.tunnel.mobile.protocol

import com.lsyl.tunnel.mobile.profile.ForwardConfig
import java.net.Socket

interface TunnelProtocol {
    fun health(): OpenResponse
    fun forwardCheck(forward: ForwardConfig): OpenResponse
    fun open(forward: ForwardConfig): Socket
}
