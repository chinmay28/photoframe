package io.github.chinmay28.photoframe

import android.app.Activity
import android.content.Context
import android.content.Intent
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.widget.Toast
import io.github.chinmay28.photoframe.core.Control
import io.github.chinmay28.photoframe.core.FrameApi
import java.net.HttpURLConnection
import kotlin.concurrent.thread

/**
 * The launcher shortcuts' Next photo, Previous and Pause / resume — the
 * frame's A, B and C buttons — sent without opening the app. Shows nothing
 * but a toast: it hands the command to a thread and is gone.
 */
class ControlActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val control = Control.of(intent?.action)
        val config = ConfigStore(this).load()
        when {
            control == null -> Unit
            !config.configured -> {
                Toast.makeText(this, R.string.not_set_up, Toast.LENGTH_LONG).show()
                startActivity(Intent(this, MainActivity::class.java))
            }
            else -> send(applicationContext, control)
        }
        finish()
    }

    private fun send(app: Context, control: Control) {
        val main = Handler(Looper.getMainLooper())
        val toast = { text: String -> main.post { Toast.makeText(app, text, Toast.LENGTH_SHORT).show() } }
        thread(name = "photoframe-control") {
            try {
                val origin = FrameResolver.resolve(app, ConfigStore(app).load().origin().getOrThrow()).getOrThrow()
                val paused = if (control.needsStatus) {
                    val r = FrameHttp.get(origin, FrameApi.STATUS)
                    FrameApi.status(r.body)?.paused ?: throw IllegalStateException(FrameApi.error(r.body) ?: "the frame sent no status")
                } else {
                    false
                }
                val command = control.command(paused)
                val r = FrameHttp.post(origin, FrameApi.control(command))
                toast(
                    when (r.code) {
                        HttpURLConnection.HTTP_ACCEPTED, HttpURLConnection.HTTP_OK -> app.getString(
                            when (command) {
                                "next" -> R.string.control_next
                                "prev" -> R.string.control_prev
                                "pause" -> R.string.control_pause
                                else -> R.string.control_resume
                            },
                        )
                        HttpURLConnection.HTTP_UNAVAILABLE -> app.getString(R.string.control_busy)
                        else -> app.getString(R.string.control_failed, FrameApi.error(r.body) ?: "HTTP ${r.code}")
                    },
                )
            } catch (e: FrameResolver.NotFound) {
                toast(app.getString(R.string.control_failed, app.getString(R.string.cannot_find, e.host)))
            } catch (e: Exception) {
                toast(app.getString(R.string.control_failed, FrameHttp.describe(e)))
            }
        }
    }
}
