/* Apparitions au scroll, sans lib et sans risque : la page reste entière si
   ce script ne tourne pas — le masquage ne s'applique que sous la classe
   js-reveal que ce fichier pose quand l'observer existe, et le mouvement est
   rogné au complet quand le système demande moins d'animations. */

(function () {
  'use strict';

  const targets = document.querySelectorAll('.reveal');
  if (!targets.length) return;

  const reduce = typeof matchMedia === 'function' &&
    matchMedia('(prefers-reduced-motion: reduce)').matches;
  if (reduce || !('IntersectionObserver' in window)) {
    for (const node of targets) node.classList.add('is-in');
    return; // visible d'un bloc, sans transition : la lecture passe avant
  }

  // La classe devenue maître du masquage est posée avant l'observation, pour
  // que nul contenu n'apparaisse déjà vu puis replonge : ce qui est déjà à
  // l'écran au premier rendu reçoit is-in à la première tâche de l'observer.
  document.documentElement.classList.add('js-reveal');
  const io = new IntersectionObserver((entries) => {
    for (const entry of entries) {
      if (!entry.isIntersecting) continue;
      entry.target.classList.add('is-in');
      io.unobserve(entry.target);
    }
  }, { rootMargin: '0px 0px -10% 0px' });
  for (const node of targets) io.observe(node);
})();
