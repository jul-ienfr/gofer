(function () {
  "use strict"

  function readSettings(form) {
    var settings = {}
    var inputs = form.querySelectorAll("[data-ai-setting]")
    for (var i = 0; i < inputs.length; i++) {
      settings[inputs[i].getAttribute("data-ai-setting")] = inputs[i].checked
    }
    return settings
  }

  function setStatus(form, message, isError) {
    var status = form.querySelector("[data-ai-settings-status]")
    if (!status) return
    status.textContent = message
    status.classList.toggle("text-red-700", !!isError)
    status.classList.toggle("dark:text-red-300", !!isError)
  }

  function t(key) {
    return window.goferT ? window.goferT(key) : key
  }

  function save(form) {
    var button = form.querySelector("[data-ai-settings-save]")
    if (button) button.disabled = true
    setStatus(form, t("js.settings.status.saving"), false)
    fetch("/api/ai/settings", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify(readSettings(form)),
    })
      .then(function (response) {
        if (!response.ok) {
          return response.text().then(function (text) {
            throw new Error((text || t("js.settings.status.save_failed")).trim())
          })
        }
        return response.json()
      })
      .then(function () {
        setStatus(form, t("js.settings.status.saved"), false)
      })
      .catch(function (error) {
        setStatus(form, error.message || t("js.settings.status.save_failed"), true)
      })
      .then(function () {
        if (button) button.disabled = false
      })
  }

  document.addEventListener("submit", function (event) {
    var form = event.target && event.target.closest
      ? event.target.closest("[data-ai-settings-form]")
      : null
    if (!form) return
    event.preventDefault()
    save(form)
  })
})()
