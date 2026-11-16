package io.github.chinmay28.photoframe

import android.animation.ValueAnimator
import android.app.Activity
import android.app.Dialog
import android.graphics.Typeface
import android.util.TypedValue
import android.view.Gravity
import android.view.ViewGroup
import android.view.WindowManager
import android.view.animation.LinearInterpolator
import android.widget.FrameLayout
import android.widget.LinearLayout
import android.widget.TextView
import com.google.android.material.imageview.ShapeableImageView
import com.google.android.material.shape.ShapeAppearanceModel
import io.github.chinmay28.photoframe.core.DevFlash
import kotlin.math.min

/**
 * The developer badge thrown up full screen for a moment, as the web app's
 * About card does (showDevFlash in web/app.js): the round badge with
 * github.com/chinmay28 under it in monospace, over a dark veil, for 3 s.
 * A tap anywhere ends it early. The timeline is [DevFlash].
 */
object DevBadge {
    fun flash(activity: Activity) {
        val dialog = Dialog(activity, R.style.Theme_Photoframe_DevFlash)
        val density = activity.resources.displayMetrics.density
        val metrics = activity.resources.displayMetrics
        // min(64vmin, 26rem), as .dev-lockup img.
        val side = min(0.64f * min(metrics.widthPixels, metrics.heightPixels), 416 * density).toInt()

        val veil = FrameLayout(activity).apply {
            setBackgroundColor(activity.getColor(R.color.dev_veil))
            alpha = 0f
        }
        val lockup = LinearLayout(activity).apply {
            orientation = LinearLayout.VERTICAL
            gravity = Gravity.CENTER_HORIZONTAL
        }
        val badge = ShapeableImageView(activity).apply {
            setImageResource(R.drawable.dev_badge_full)
            contentDescription = activity.getString(R.string.dev_badge_full)
            shapeAppearanceModel = ShapeAppearanceModel.builder().setAllCornerSizes(side / 2f).build()
            elevation = 24 * density
        }
        val link = TextView(activity).apply {
            setText(R.string.dev_link)
            typeface = Typeface.MONOSPACE
            letterSpacing = 0.08f
            setTextColor(activity.getColor(R.color.dev_text))
            setTextSize(TypedValue.COMPLEX_UNIT_SP, 15f)
            gravity = Gravity.CENTER
        }
        lockup.addView(badge, LinearLayout.LayoutParams(side, side))
        lockup.addView(link, LinearLayout.LayoutParams(ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT).apply {
            topMargin = (22 * density).toInt()
            gravity = Gravity.CENTER_HORIZONTAL
        })
        veil.addView(lockup, FrameLayout.LayoutParams(ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT, Gravity.CENTER))
        veil.setOnClickListener { dialog.dismiss() }

        dialog.setContentView(veil)
        dialog.window?.apply {
            setLayout(WindowManager.LayoutParams.MATCH_PARENT, WindowManager.LayoutParams.MATCH_PARENT)
            setBackgroundDrawableResource(android.R.color.transparent)
        }

        // With animations off in the phone's settings an animator jumps
        // straight to its end, so the badge is shown still and timed instead
        // — the reduced-motion case the CSS has too.
        val animate = ValueAnimator.areAnimatorsEnabled()
        val animator = ValueAnimator.ofFloat(0f, 1f).apply {
            duration = DevFlash.DURATION_MS
            interpolator = LinearInterpolator()
            addUpdateListener {
                val t = it.animatedValue as Float
                veil.alpha = DevFlash.veilAlpha(t)
                val s = DevFlash.badgeScale(t)
                lockup.scaleX = s
                lockup.scaleY = s
            }
        }
        val end = Runnable { if (dialog.isShowing) dialog.dismiss() }
        dialog.setOnDismissListener {
            animator.cancel()
            veil.removeCallbacks(end)
        }
        dialog.show()
        if (animate) {
            animator.start()
        } else {
            veil.alpha = 1f
        }
        veil.postDelayed(end, DevFlash.DURATION_MS)
    }
}
