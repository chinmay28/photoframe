package io.github.chinmay28.photoframe

import android.content.Intent
import android.os.Bundle
import android.widget.LinearLayout
import android.widget.TextView
import androidx.activity.enableEdgeToEdge
import androidx.appcompat.app.AppCompatActivity
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.widget.doAfterTextChanged
import com.google.android.material.button.MaterialButton
import com.google.android.material.textfield.TextInputEditText
import com.google.android.material.textfield.TextInputLayout
import io.github.chinmay28.photoframe.core.AppConfig
import io.github.chinmay28.photoframe.core.Discovery
import io.github.chinmay28.photoframe.core.Origin
import kotlin.concurrent.thread

/**
 * The first screen: which frame. It starts on photoframe.local, the name the
 * README installs a frame under, so with one frame on the Wi-Fi it is a
 * single tap; **Find on this network** lists the frames that announce
 * themselves.
 */
class SetupActivity : AppCompatActivity() {
    private lateinit var addressLayout: TextInputLayout
    private lateinit var address: TextInputEditText
    private lateinit var found: LinearLayout
    private lateinit var find: MaterialButton
    private lateinit var go: MaterialButton

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_setup)
        ViewCompat.setOnApplyWindowInsetsListener(findViewById(R.id.root)) { v, insets ->
            val bars = insets.getInsets(WindowInsetsCompat.Type.systemBars() or WindowInsetsCompat.Type.displayCutout() or WindowInsetsCompat.Type.ime())
            v.setPadding(bars.left, bars.top, bars.right, bars.bottom)
            WindowInsetsCompat.CONSUMED
        }

        addressLayout = findViewById(R.id.address_layout)
        address = findViewById(R.id.address)
        found = findViewById(R.id.found)
        find = findViewById(R.id.find)
        go = findViewById(R.id.go)
        findViewById<TextView>(R.id.version).text = getString(R.string.setup_version, BuildConfig.VERSION_NAME)
        findViewById<android.view.View>(R.id.devmark).setOnClickListener { DevBadge.flash(this) }

        val store = ConfigStore(this)
        address.setText(store.load().frameAddress.ifBlank { Origin.DEFAULT_ADDRESS })
        go.isEnabled = !address.text.isNullOrBlank()

        address.doAfterTextChanged {
            addressLayout.error = null
            go.isEnabled = !it.isNullOrBlank()
        }
        find.setOnClickListener { discover() }
        go.setOnClickListener {
            val o = Origin.parse(address.text.toString()).getOrElse {
                addressLayout.error = it.message
                return@setOnClickListener
            }
            store.save(AppConfig(frameAddress = o.toString()))
            startActivity(Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_CLEAR_TOP))
            finish()
        }
    }

    private fun discover() {
        find.isEnabled = false
        find.setText(R.string.setup_finding)
        found.removeAllViews()
        thread(name = "photoframe-discovery") {
            val frames = FrameDiscovery.find(this)
            runOnUiThread {
                if (isDestroyed) return@runOnUiThread
                find.isEnabled = true
                find.setText(R.string.setup_find)
                showFound(frames)
            }
        }
    }

    private fun showFound(frames: List<Discovery.Found>) {
        found.removeAllViews()
        if (frames.isEmpty()) {
            found.addView(TextView(this).apply {
                setText(R.string.setup_found_none)
                setTextColor(getColor(R.color.pf_muted))
            })
            return
        }
        for (f in frames) {
            found.addView(MaterialButton(this, null, com.google.android.material.R.attr.materialButtonOutlinedStyle).apply {
                text = getString(R.string.setup_found_item, f.name, f.origin.host)
                isAllCaps = false
                setOnClickListener { address.setText(f.address) }
            })
        }
    }
}
