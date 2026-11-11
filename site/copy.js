for (const b of document.querySelectorAll('[data-copy]')) {
  b.addEventListener('click', () => {
    void navigator.clipboard.writeText(b.dataset.copy).then(() => {
      b.textContent = 'Copied'
      setTimeout(() => {
        b.textContent = 'Copy'
      }, 1500)
    })
  })
}
