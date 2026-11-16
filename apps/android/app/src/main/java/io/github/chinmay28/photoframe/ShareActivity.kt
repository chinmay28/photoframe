package io.github.chinmay28.photoframe

import android.content.Intent
import android.graphics.Bitmap
import android.graphics.ImageDecoder
import android.net.Uri
import android.os.Bundle
import android.provider.OpenableColumns
import android.view.ViewGroup
import android.widget.LinearLayout
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AlertDialog
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.IntentCompat
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import com.google.android.material.progressindicator.LinearProgressIndicator
import io.github.chinmay28.photoframe.core.FrameApi
import io.github.chinmay28.photoframe.core.Multipart
import io.github.chinmay28.photoframe.core.Origin
import io.github.chinmay28.photoframe.core.ShareInput
import io.github.chinmay28.photoframe.core.UploadAs
import io.github.chinmay28.photoframe.core.UploadPlan
import java.io.File
import java.io.InputStream
import kotlin.concurrent.thread

/**
 * "Photoframe" in the share sheet: the shared photos go to the frame's
 * upload source through the same `POST /api/sources/{id}/photos` the page's
 * Upload button uses, in one request, so the frame rescans once.
 *
 * Photos the frame can't decode — HEIC above all, which is what many phone
 * cameras save — are turned into JPEGs here first ([UploadPlan]), which a
 * browser upload can't do. Everything decided is in :core; this is the
 * plumbing between the share sheet, the content resolver and the frame.
 */
class ShareActivity : AppCompatActivity() {
    private lateinit var dialog: AlertDialog
    private lateinit var text: TextView
    private lateinit var bar: LinearProgressIndicator
    @Volatile private var cancelled = false

    private class Item(val uri: Uri, val name: String, val plan: UploadAs, val mime: String, var file: File? = null)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val config = ConfigStore(this).load()
        if (!config.configured) {
            Toast.makeText(this, R.string.not_set_up, Toast.LENGTH_LONG).show()
            startActivity(Intent(this, MainActivity::class.java))
            finish()
            return
        }
        val uris = ShareInput.accept(sharedUris().map { it.toString() }).map(Uri::parse)
        if (uris.isEmpty()) {
            finishWith(getString(R.string.share_none), openApp = false)
            return
        }
        showProgress()
        thread(name = "photoframe-share") { send(config.origin().getOrThrow(), uris) }
    }

    private fun sharedUris(): List<Uri> {
        val i = intent ?: return emptyList()
        val out = ArrayList<Uri>()
        when (i.action) {
            Intent.ACTION_SEND -> IntentCompat.getParcelableExtra(i, Intent.EXTRA_STREAM, Uri::class.java)?.let { out += it }
            Intent.ACTION_SEND_MULTIPLE -> IntentCompat.getParcelableArrayListExtra(i, Intent.EXTRA_STREAM, Uri::class.java)?.let { out += it }
        }
        val clip = i.clipData
        if (out.isEmpty() && clip != null) for (k in 0 until clip.itemCount) clip.getItemAt(k).uri?.let { out += it }
        return out
    }

    private fun send(configured: Origin, uris: List<Uri>) {
        val leftOut = ArrayList<String>()
        val items = ArrayList<Item>()
        try {
            val origin = FrameResolver.resolve(this, configured).getOrElse {
                return finishWith(getString(R.string.cannot_find, configured.host) + "\n\n" + getString(R.string.cannot_find_detail), openApp = false)
            }
            val sources = FrameHttp.get(origin, FrameApi.SOURCES)
            val target = FrameApi.uploadSource(sources.body)
                ?: return finishWith(FrameApi.error(sources.body)?.let { getString(R.string.share_failed, it) } ?: getString(R.string.share_no_source), openApp = true)

            uris.forEachIndexed { i, uri ->
                val shownName = displayName(uri)
                val mime = contentResolver.getType(uri) ?: intent?.type
                val plan = UploadPlan.decide(shownName, mime)
                if (plan == UploadAs.REFUSE) {
                    leftOut += "${shownName ?: uri.lastPathSegment}: ${getString(R.string.share_not_image)}"
                } else {
                    items += Item(uri, UploadPlan.nameFor(shownName, mime, plan, i), plan, UploadPlan.mimeFor(shownName, mime, plan))
                }
            }
            if (items.isEmpty()) return finishWith(getString(R.string.share_none) + leftOutText(leftOut), openApp = false)

            // Conversions first, so the frame never waits on the phone
            // mid-request.
            for (item in items.filter { it.plan == UploadAs.JPEG }) {
                if (cancelled) return
                progress(getString(R.string.share_converting, item.name), null)
                item.file = runCatching { toJpeg(item.uri) }.getOrNull()
                if (item.file == null) leftOut += "${item.name}: ${getString(R.string.share_unreadable)}"
            }
            val ready = items.filter { it.plan == UploadAs.AS_IS || it.file != null }
            if (ready.isEmpty()) return finishWith(getString(R.string.share_none) + leftOutText(leftOut), openApp = false)

            val mp = Multipart()
            // The frame rescans the source before it answers.
            val c = FrameHttp.open(origin, FrameApi.upload(target.id), "POST", readTimeoutMs = 180_000)
            c.doOutput = true
            c.setChunkedStreamingMode(64 * 1024)
            c.setRequestProperty("Content-Type", mp.contentType)
            c.outputStream.buffered(64 * 1024).use { out ->
                ready.forEachIndexed { i, item ->
                    if (cancelled) {
                        c.disconnect()
                        return
                    }
                    progress(getString(R.string.share_sending, i + 1, ready.size), (i * 100) / ready.size)
                    val input: InputStream? = item.file?.inputStream() ?: runCatching { contentResolver.openInputStream(item.uri) }.getOrNull()
                    if (input == null) {
                        leftOut += "${item.name}: ${getString(R.string.share_unreadable)}"
                    } else {
                        input.use { mp.writeFile(out, "photos", item.name, item.mime, it) }
                    }
                }
                mp.finish(out)
            }
            progress(getString(R.string.share_waiting), 100)
            val r = FrameHttp.finish(c)
            val result = FrameApi.uploadResult(r.body)
            if (result == null || (r.code != 201 && r.code != 422)) {
                return finishWith(getString(R.string.share_failed, FrameApi.error(r.body) ?: "HTTP ${r.code}"), openApp = false)
            }
            leftOut += result.rejected
            val sent = if (result.saved.isEmpty()) getString(R.string.share_none)
            else resources.getQuantityString(R.plurals.share_sent, result.saved.size, result.saved.size, target.name)
            finishWith(sent + leftOutText(leftOut), openApp = result.saved.isNotEmpty())
        } catch (e: Exception) {
            finishWith(getString(R.string.share_failed, FrameHttp.describe(e)), openApp = false)
        } finally {
            items.forEach { it.file?.delete() }
        }
    }

    private fun leftOutText(leftOut: List<String>) =
        if (leftOut.isEmpty()) "" else "\n\n" + getString(R.string.share_left_out, leftOut.joinToString("\n"))

    private fun displayName(uri: Uri): String? = runCatching {
        contentResolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)?.use { c ->
            if (c.moveToFirst()) c.getString(0) else null
        }
    }.getOrNull() ?: uri.lastPathSegment

    /** Decodes any image Android can (HEIC included) and writes it as a JPEG. */
    private fun toJpeg(uri: Uri): File {
        val bmp = ImageDecoder.decodeBitmap(ImageDecoder.createSource(contentResolver, uri)) { decoder, info, _ ->
            val (w, h) = UploadPlan.convertedSize(info.size.width, info.size.height)
            if (w != info.size.width || h != info.size.height) decoder.setTargetSize(w, h)
            decoder.allocator = ImageDecoder.ALLOCATOR_SOFTWARE
        }
        val dir = File(cacheDir, "share").apply { mkdirs() }
        val file = File.createTempFile("photo", ".jpg", dir)
        try {
            file.outputStream().buffered().use { bmp.compress(Bitmap.CompressFormat.JPEG, 92, it) }
        } catch (e: Exception) {
            file.delete()
            throw e
        } finally {
            bmp.recycle()
        }
        return file
    }

    // --- the dialog ---

    private fun showProgress() {
        val pad = (24 * resources.displayMetrics.density).toInt()
        text = TextView(this).apply { setText(R.string.share_preparing) }
        bar = LinearProgressIndicator(this).apply { isIndeterminate = true }
        val box = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(pad, pad / 2, pad, 0)
            addView(text)
            addView(bar, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT).apply { topMargin = pad / 2 })
        }
        dialog = MaterialAlertDialogBuilder(this)
            .setTitle(R.string.share_title)
            .setView(box)
            .setNegativeButton(android.R.string.cancel) { _, _ -> cancelled = true }
            .setOnDismissListener { if (!isChangingConfigurations) finish() }
            .show()
    }

    private fun progress(message: String, percent: Int?) = runOnUiThread {
        if (isFinishing) return@runOnUiThread
        text.text = message
        if (percent == null) {
            bar.isIndeterminate = true
        } else {
            bar.isIndeterminate = false
            bar.setProgressCompat(percent, true)
        }
    }

    private fun finishWith(message: String, openApp: Boolean) = runOnUiThread {
        if (isDestroyed || cancelled) {
            finish()
            return@runOnUiThread
        }
        if (::dialog.isInitialized) {
            dialog.setOnDismissListener(null)
            dialog.dismiss()
        }
        val b = MaterialAlertDialogBuilder(this)
            .setTitle(R.string.share_title)
            .setMessage(message)
            .setPositiveButton(R.string.share_done, null)
            .setOnDismissListener { finish() }
        if (openApp) {
            b.setNeutralButton(R.string.share_open_app) { _, _ ->
                startActivity(Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
            }
        }
        b.show()
    }
}
