package com.phoring.fenghuangming.player

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Intent
import android.content.pm.ServiceInfo
import android.media.AudioManager
import android.net.Uri
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.os.ParcelFileDescriptor
import android.os.SystemClock
import androidx.media3.common.AudioAttributes
import androidx.media3.common.C
import androidx.media3.common.ForwardingPlayer
import androidx.media3.common.MediaItem
import androidx.media3.common.MediaMetadata
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.common.util.UnstableApi
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.session.MediaSession
import androidx.media3.session.MediaSessionService
import org.json.JSONObject
import java.io.File

@androidx.annotation.OptIn(UnstableApi::class)
class PlaybackService : MediaSessionService() {
    private val main = Handler(Looper.getMainLooper())
    private val gate = PlaybackGate()
    private lateinit var player: ExoPlayer
    private lateinit var session: MediaSession
    private lateinit var bridge: RootBridge
    private var audioFile: ParcelFileDescriptor? = null
    private var started = false
    private var preparedAt = 0L
    private var title = "等待点歌"
    private var destroyed = false
    private var foregroundReady = false
    private var lastError = ""
    private lateinit var incomingAudio: File

    override fun onCreate() {
        super.onCreate()
        android.util.Log.i("FhmPlayer", "创建原生播放服务")
        val directory = File(cacheDir, "fenghuangming")
        check(directory.mkdirs() || directory.isDirectory)
        android.system.Os.chmod(directory.absolutePath, 448)
        incomingAudio = File(directory, "incoming.audio")
        val incoming = android.system.Os.open(incomingAudio.absolutePath,
            android.system.OsConstants.O_WRONLY or android.system.OsConstants.O_CREAT or android.system.OsConstants.O_TRUNC or android.system.OsConstants.O_NOFOLLOW, 384)
        android.system.Os.close(incoming)
        android.system.Os.chmod(incomingAudio.absolutePath, 384)
        val notifications = getSystemService(NotificationManager::class.java)
        notifications.createNotificationChannel(NotificationChannel("playback", "凤凰鸣播放服务", NotificationManager.IMPORTANCE_LOW))
        player = ExoPlayer.Builder(this).build().apply {
            setAudioAttributes(AudioAttributes.Builder().setUsage(C.USAGE_MEDIA).setContentType(C.AUDIO_CONTENT_TYPE_MUSIC).build(), true)
            setHandleAudioBecomingNoisy(true)
            setWakeMode(C.WAKE_MODE_LOCAL)
            addListener(object : Player.Listener {
                override fun onIsPlayingChanged(isPlaying: Boolean) {
                    if (isPlaying && !started && gate.playbackId != null) { started = true; event("started") }
                    status(); postNotification()
                }
                override fun onPlaybackStateChanged(playbackState: Int) {
                    if (playbackState == Player.STATE_ENDED) stopCurrent("ended")
                    status()
                }
                override fun onPlayerError(error: PlaybackException) {
                    android.util.Log.e("FhmPlayer", "原生播放失败：${error.errorCodeName}", error)
                    lastError = error.errorCodeName
                    stopCurrent("failed", error.errorCodeName)
                }
            })
        }
        val controlled = object : ForwardingPlayer(player) {
            override fun play() { if (gate.canPlay(SystemClock.elapsedRealtime())) super.play() }
            override fun setPlayWhenReady(playWhenReady: Boolean) {
                if (!playWhenReady || gate.canPlay(SystemClock.elapsedRealtime())) super.setPlayWhenReady(playWhenReady)
            }
            override fun seekTo(positionMs: Long) {
                if (gate.canPlay(SystemClock.elapsedRealtime())) super.seekTo(positionMs / 1000 * 1000)
            }
            override fun stop() { stopCurrent("stopped") }
        }
        session = MediaSession.Builder(this, controlled).setCallback(object : MediaSession.Callback {
            override fun onConnect(session: MediaSession, controller: MediaSession.ControllerInfo): MediaSession.ConnectionResult {
                if (!controller.isTrusted && controller.uid != android.os.Process.myUid()) return MediaSession.ConnectionResult.reject()
                val commands = Player.Commands.Builder().addAll(
                    Player.COMMAND_PLAY_PAUSE, Player.COMMAND_STOP, Player.COMMAND_SEEK_IN_CURRENT_MEDIA_ITEM,
                    Player.COMMAND_GET_CURRENT_MEDIA_ITEM, Player.COMMAND_GET_METADATA, Player.COMMAND_GET_TIMELINE,
                    Player.COMMAND_GET_VOLUME, Player.COMMAND_GET_AUDIO_ATTRIBUTES,
                ).build()
                return MediaSession.ConnectionResult.AcceptedResultBuilder(session).setAvailablePlayerCommands(commands).build()
            }
        }).build()
        postNotification()
        bridge = RootBridge({ message -> main.post { handle(message) } }, {
            main.post { gate.blocked = true; gate.leaseUntil = 0; stopCurrent("stopped") }
        })
        main.post(tick)
    }
    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == "local-stop") stopCurrent("stopped")
        postNotification()
        super.onStartCommand(intent, flags, startId)
        return START_STICKY
    }
    override fun onGetSession(controllerInfo: MediaSession.ControllerInfo): MediaSession? =
        if (controllerInfo.isTrusted || controllerInfo.uid == android.os.Process.myUid()) session else null

    override fun onUpdateNotification(session: MediaSession, startInForegroundRequired: Boolean) { postNotification() }

    private fun postNotification() {
        if (!::player.isInitialized || destroyed) return
        val open = PendingIntent.getActivity(this, 1, Intent(this, BootstrapActivity::class.java), PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
        val stop = PendingIntent.getService(this, 2, Intent(this, PlaybackService::class.java).setAction("local-stop"), PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
        val text = when { gate.blocked -> "本机已制动"; player.isPlaying -> "正在播放"; gate.playbackId != null -> "暂停或准备中"; else -> "已启动，等待后端指令" }
        val notification = Notification.Builder(this, "playback").setSmallIcon(android.R.drawable.ic_media_play)
            .setContentTitle("凤凰鸣 · $title").setContentText(text).setContentIntent(open).setOngoing(true)
            .addAction(Notification.Action.Builder(null, "停止播放", stop).build()).build()
        try {
            val type = if (Build.VERSION.SDK_INT >= 34) ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE or
                (if (gate.playbackId != null) ServiceInfo.FOREGROUND_SERVICE_TYPE_MEDIA_PLAYBACK else 0)
            else ServiceInfo.FOREGROUND_SERVICE_TYPE_MEDIA_PLAYBACK
            startForeground(2001, notification, type)
            foregroundReady = true
        } catch (error: Exception) {
            foregroundReady = false
            lastError = "${error.javaClass.simpleName}: ${error.message.orEmpty()}".take(400)
            android.util.Log.e("FhmPlayer", "前台服务启动失败", error)
        }
    }
    private fun handle(message: JSONObject) {
        if (destroyed) return
        val requestId = message.optString("requestId")
        var opened: ParcelFileDescriptor? = null
        try {
            if (message.optString("type") == "lease") {
                gate.renew(message.optLong("epoch"), message.optLong("leaseMs"), SystemClock.elapsedRealtime())
                return
            }
            val action = message.getString("action")
            when (action) {
                "brake" -> {
                    gate.blocked = message.getBoolean("blocked")
                    if (gate.blocked) stopCurrent("stopped")
                }
                "start" -> {
                    require(!gate.blocked && SystemClock.elapsedRealtime() < gate.leaseUntil && foregroundReady) { "播放器未就绪或已制动" }
                    require(message.getLong("epoch") == gate.epoch) { "连接已改变" }
                    val descriptor = ParcelFileDescriptor.open(incomingAudio, ParcelFileDescriptor.MODE_READ_ONLY)
                    opened = descriptor
                    require(android.system.Os.fstat(descriptor.fileDescriptor).st_size == message.getLong("bytes")) { "音频文件无效" }
                    stopCurrent("stopped")
                    gate.playbackId = message.getString("playbackId")
                    title = message.optString("title", "正在播放")
                    lastError = ""
                    audioFile = descriptor
                    val artists = message.optJSONArray("artists")?.let { a -> (0 until a.length()).joinToString(" / ") { a.optString(it) } } ?: ""
                    val item = MediaItem.Builder().setMediaId(gate.playbackId!!).setUri(Uri.fromFile(File("/proc/self/fd/${descriptor.fd}")))
                        .setMediaMetadata(MediaMetadata.Builder().setTitle(title).setArtist(artists).build()).build()
                    player.volume = message.getInt("volume").coerceIn(0, 100) / 100f
                    preparedAt = SystemClock.elapsedRealtime()
                    started = false
                    player.setMediaItem(item); player.prepare(); player.play()
                    postNotification()
                }
                "stop" -> if (message.optString("playbackId") == gate.playbackId) stopCurrent("stopped")
                "pause", "resume", "seek" -> {
                    gate.requireTask(message.getString("playbackId"), message.getLong("epoch"), SystemClock.elapsedRealtime())
                    when (action) {
                        "pause" -> player.pause()
                        "resume" -> player.play()
                        "seek" -> {
                            require(started && player.isCurrentMediaItemSeekable) { "当前歌曲不可跳转" }
                            val paused = !player.playWhenReady
                            player.seekTo(gate.seekMillis(message.getDouble("position"), duration()))
                            player.playWhenReady = !paused
                        }
                    }
                }
                "volume" -> {
                    require(!gate.blocked && SystemClock.elapsedRealtime() < gate.leaseUntil) { "连接租约已失效" }
                    val volume = message.getInt("volume"); require(volume in 0..100)
                    player.volume = volume / 100f
                }
                else -> error("不支持的本机指令")
            }
            if (requestId.isNotEmpty()) bridge.send(JSONObject().put("type", "result").put("requestId", requestId).put("ok", true))
            status()
        } catch (error: Exception) {
            if (opened !== audioFile) opened?.close()
            lastError = error.message?.take(200) ?: "播放控制失败"
            if (message.optString("action") == "start" && opened === audioFile && gate.playbackId == message.optString("playbackId")) stopCurrent("failed", lastError)
            if (requestId.isNotEmpty()) bridge.send(JSONObject().put("type", "result").put("requestId", requestId).put("ok", false).put("error", lastError))
        }
    }
    private fun duration(): Long = player.duration.takeIf { it > 0 && it != C.TIME_UNSET } ?: 0
    private fun event(state: String, error: String = "") {
        if (!::bridge.isInitialized) return
        val id = gate.playbackId ?: return
        bridge.send(JSONObject().put("type", "event").put("playbackId", id).put("event", state).put("error", error))
    }
    private fun stopCurrent(state: String, error: String = "") {
        val id = gate.playbackId
        gate.playbackId = null
        if (::player.isInitialized) { player.stop(); player.clearMediaItems() }
        audioFile?.close(); audioFile = null; started = false; preparedAt = 0
        title = "等待点歌"
        if (id != null && ::bridge.isInitialized) bridge.send(JSONObject().put("type", "event").put("playbackId", id).put("event", state).put("error", error))
    }
    private fun status() {
        if (!::bridge.isInitialized || destroyed) return
        val manager = getSystemService(AudioManager::class.java)
        val outputs = manager.getDevices(AudioManager.GET_DEVICES_OUTPUTS).joinToString(" / ") { it.productName.toString() }
        val state = JSONObject().put("ready", foregroundReady).put("playbackId", gate.playbackId ?: JSONObject.NULL)
            .put("started", started).put("position", player.currentPosition.coerceAtLeast(0) / 1000.0)
            .put("durationSeconds", duration() / 1000.0).put("seekable", player.isCurrentMediaItemSeekable)
            .put("paused", !player.playWhenReady).put("output", outputs).put("systemVolume", manager.getStreamVolume(AudioManager.STREAM_MUSIC)).put("error", lastError)
        publishLocalStatus(foregroundReady)
        bridge.send(JSONObject().put("type", "status").put("state", state))
    }
    private fun publishLocalStatus(ready: Boolean) {
        try {
            val file = File(filesDir, "player-status.json")
            val temporary = File(filesDir, "player-status.tmp")
            temporary.writeText(JSONObject().put("ready", ready).put("blocked", gate.blocked)
                .put("updatedElapsed", SystemClock.elapsedRealtime()).put("error", lastError).toString())
            if (!temporary.renameTo(file)) temporary.delete()
        } catch (error: Exception) { android.util.Log.w("FhmPlayer", "无法保存本机服务状态", error) }
    }
    private val tick = object : Runnable {
        override fun run() {
            if (destroyed) return
            val now = SystemClock.elapsedRealtime()
            if (gate.playbackId != null && (gate.blocked || now >= gate.leaseUntil)) stopCurrent("stopped")
            if (gate.playbackId != null && !started && now - preparedAt > 30000) stopCurrent("failed", "准备音频或获取音频焦点超时")
            status(); main.postDelayed(this, 1000)
        }
    }
    override fun onDestroy() {
        destroyed = true
        foregroundReady = false
        publishLocalStatus(false)
        android.util.Log.i("FhmPlayer", "销毁原生播放服务")
        main.removeCallbacksAndMessages(null)
        stopCurrent("stopped")
        if (::bridge.isInitialized) bridge.close()
        if (::session.isInitialized) session.release()
        if (::player.isInitialized) player.release()
        if (::incomingAudio.isInitialized) incomingAudio.delete()
        super.onDestroy()
    }
}
