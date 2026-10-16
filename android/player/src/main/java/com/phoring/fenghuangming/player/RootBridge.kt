package com.phoring.fenghuangming.player

import android.net.LocalServerSocket
import android.net.LocalSocket
import android.os.Process
import org.json.JSONObject
import java.io.ByteArrayOutputStream
import java.io.FileDescriptor
import java.util.concurrent.atomic.AtomicBoolean

internal class RootBridge(
    private val receive: (JSONObject) -> Unit,
    private val disconnected: () -> Unit,
) : AutoCloseable {
    private val running = AtomicBoolean(true)
    private val server = LocalServerSocket("fenghuangming.player.v1.${Process.myUid()}")
    private val writeLock = Any()
    @Volatile private var client: LocalSocket? = null
    init {
        Thread({
            while (running.get()) {
                try {
                    val socket = server.accept()
                    if (socket.peerCredentials.uid != 0) { socket.close(); continue }
                    client = socket
                    try { read(socket) } finally { client = null; socket.close(); disconnected() }
                } catch (_: Exception) { if (running.get()) Thread.sleep(1000) }
            }
        }, "fhm-root-bridge").start()
    }
    private fun closeDescriptors(descriptors: List<FileDescriptor>) {
        descriptors.forEach { try { android.system.Os.close(it) } catch (_: Exception) {} }
    }
    private fun read(socket: LocalSocket) {
        val input = socket.inputStream
        val line = ByteArrayOutputStream()
        val descriptors = mutableListOf<FileDescriptor>()
        try {
            while (running.get()) {
                val byte = input.read()
                if (byte < 0) return
                socket.ancillaryFileDescriptors?.let { descriptors.addAll(it) }
                require(descriptors.size <= 1) { "文件描述符数量无效" }
                if (byte == 10) {
                    val message = JSONObject(line.toString(Charsets.UTF_8.name()))
                    line.reset()
                    require(descriptors.isEmpty()) { "控制消息不接受外部文件描述符" }
                    closeDescriptors(descriptors); descriptors.clear()
                    receive(message)
                } else {
                    require(line.size() < 65536) { "本机消息过大" }
                    line.write(byte)
                }
            }
        } finally { closeDescriptors(descriptors) }
    }
    fun send(message: JSONObject) = synchronized(writeLock) {
        try { client?.outputStream?.apply { write((message.toString() + "\n").toByteArray(Charsets.UTF_8)); flush() } } catch (_: Exception) { client?.close() }
    }
    override fun close() {
        running.set(false)
        try { client?.close() } catch (_: Exception) {}
        try { server.close() } catch (_: Exception) {}
    }
}
