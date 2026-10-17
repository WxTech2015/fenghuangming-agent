package com.phoring.fenghuangming.player

import android.app.Activity
import android.content.Intent
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import android.graphics.Color
import android.view.Gravity
import android.widget.Button
import android.widget.LinearLayout
import android.widget.TextView
import org.json.JSONObject
import java.io.File

class BootstrapActivity : Activity() {
    private val main = Handler(Looper.getMainLooper())
    private lateinit var status: TextView
    private lateinit var detail: TextView
    private var requestedAt = 0L
    private var startError = ""

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val padding = (24 * resources.displayMetrics.density).toInt()
        val layout = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            gravity = Gravity.CENTER_VERTICAL
            setPadding(padding, padding, padding, padding)
            setBackgroundColor(Color.rgb(16, 20, 33))
        }
        fun text(value: String, size: Float, color: Int) = TextView(this).apply {
            text = value; textSize = size; setTextColor(color)
            setPadding(0, padding / 2, 0, padding / 2)
            layout.addView(this)
        }
        text("凤凰鸣 · 播放服务", 26f, Color.WHITE)
        status = text("正在启动…", 21f, Color.rgb(190, 176, 255))
        detail = text("连接设置、播放控制和紧急制动请使用 SukiSU 模块 WebUI。", 16f, Color.LTGRAY)
        layout.addView(Button(this).apply { text = "重新启动服务"; setOnClickListener { startPlayer() } })
        layout.addView(Button(this).apply { text = "关闭此页面"; setOnClickListener { finish() } })
        text("关闭此页面不会停止播放服务。", 14f, Color.GRAY)
        setContentView(layout)
        main.post(refresh)
        if (Build.VERSION.SDK_INT >= 33 && checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS) != android.content.pm.PackageManager.PERMISSION_GRANTED) {
            requestPermissions(arrayOf(android.Manifest.permission.POST_NOTIFICATIONS), 1)
        } else startPlayer()
    }
    private fun startPlayer() {
        requestedAt = SystemClock.elapsedRealtime()
        startError = ""
        try { startForegroundService(Intent(this, PlaybackService::class.java)) }
        catch (error: Exception) {
            startError = "${error.javaClass.simpleName}: ${error.message.orEmpty()}".take(400)
            android.util.Log.e("FhmPlayer", "启动播放服务失败", error)
        }
    }
    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        startPlayer()
    }

    private val refresh = object : Runnable {
        override fun run() {
            val state = try { JSONObject(File(filesDir, "player-status.json").readText()) } catch (_: Exception) { null }
            val now = SystemClock.elapsedRealtime()
            val updated = state?.optLong("updatedElapsed") ?: 0
            val fresh = updated <= now && now - updated < 3500
            val ready = fresh && state?.optBoolean("ready") == true
            status.text = when {
                startError.isNotEmpty() -> "启动失败"
                ready -> "播放服务正在运行"
                fresh && state?.optString("error").orEmpty().isNotEmpty() -> "播放服务未就绪"
                requestedAt > 0 && now - requestedAt > 10000 -> "未收到服务就绪状态"
                else -> "正在启动…"
            }
            val error = startError.ifEmpty { if (fresh) state?.optString("error").orEmpty() else "" }
            detail.text = when {
                error.isNotEmpty() -> error + "\n请在模块 WebUI 导出日志。"
                ready -> "原生播放器已就绪。\n" + (if (state?.optBoolean("blocked") == true) "本机播放仍处于制动状态。\n" else "") +
                    "后端连接与播放控制请返回 SukiSU 模块 WebUI 查看。"
                else -> "正在等待播放服务确认。若长时间未就绪，请在模块 WebUI 查看日志。"
            }
            main.postDelayed(this, 1000)
        }
    }

    override fun onDestroy() {
        main.removeCallbacksAndMessages(null)
        super.onDestroy()
    }
}
