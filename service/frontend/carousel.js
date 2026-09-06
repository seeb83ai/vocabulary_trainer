// Generic cross-fade carousel for the landing page feature teasers.
// Operates on every [data-carousel] container the gen-landing generator
// emits (service/cmd/gen-landing) — one card's worth of markup per
// teaser folder that has 2+ images. Nothing here is per-teaser; adding a
// third or fourth screenshot to any folder just works.
//
// Behavior: cross-fades every 6s, pauses while the pointer is over the
// card, and stops autoplay for that card permanently once a visitor clicks
// a dot (their choice of image wins from then on).

const AUTOPLAY_MS = 6000;

function nextIndex(current, total) {
  return (current + 1) % total;
}

function slideOpacityClass(isActive) {
  return isActive ? 'opacity-100' : 'opacity-0';
}

function dotMarkClass(isActive) {
  return isActive ? 'bg-blue-600' : 'bg-gray-900/25';
}

function setActiveSlide(el, slides, dots, index) {
  slides.forEach((slide, i) => {
    slide.classList.remove('opacity-0', 'opacity-100');
    slide.classList.add(slideOpacityClass(i === index));
  });
  dots.forEach((dot, i) => {
    const mark = dot.querySelector('.carousel-mark');
    if (!mark) return;
    mark.classList.remove('bg-blue-600', 'bg-gray-900/25');
    mark.classList.add(dotMarkClass(i === index));
  });
}

function initCarousel(el) {
  const slides = Array.from(el.querySelectorAll('.carousel-slide'));
  const dots = Array.from(el.querySelectorAll('.carousel-dot'));
  if (slides.length < 2) return;

  let index = 0;
  let paused = false;
  let stopped = false;

  const timer = setInterval(() => {
    if (paused || stopped) return;
    index = nextIndex(index, slides.length);
    setActiveSlide(el, slides, dots, index);
  }, AUTOPLAY_MS);

  el.addEventListener('mouseenter', () => { paused = true; });
  el.addEventListener('mouseleave', () => { paused = false; });

  dots.forEach((dot, i) => {
    dot.addEventListener('click', () => {
      stopped = true;
      index = i;
      setActiveSlide(el, slides, dots, index);
    });
  });

  el.dataset.carouselTimer = String(timer);
}

document.addEventListener('DOMContentLoaded', () => {
  document.querySelectorAll('[data-carousel]').forEach(initCarousel);
});
