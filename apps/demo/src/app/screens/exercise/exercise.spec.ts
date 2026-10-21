import { TestBed } from '@angular/core/testing';
import { beforeEach, describe, expect, it } from 'vitest';
import { FakeVitals, estimate } from '../../testing/fake-vitals';
import { TARGET_RPM, VITALS_FACTORY, VitalsService } from '../../vitals.service';
import { Exercise, guideLabel } from './exercise';

describe('Exercise', () => {
  let fake: FakeVitals;
  let service: VitalsService;
  let el: HTMLElement;
  let whenStable: () => Promise<void>;

  beforeEach(async () => {
    fake = new FakeVitals();
    TestBed.configureTestingModule({ providers: [{ provide: VITALS_FACTORY, useValue: () => fake }] });
    service = TestBed.inject(VitalsService);
    await service.consentAndStart(document.createElement('video'));
    fake.emit('estimate', estimate({ hrBpm: 66, rrRpm: 6, breathPhase: 'in' }));
    const fixture = TestBed.createComponent(Exercise);
    el = fixture.nativeElement as HTMLElement;
    whenStable = () => fixture.whenStable();
    await whenStable();
  });

  function toggle(): HTMLButtonElement {
    const button = el.querySelector('button');
    if (button === null) throw new Error('no toggle');
    return button;
  }

  it('hides the numbers until the visitor asks for them', async () => {
    expect(toggle().textContent.trim()).toBe('Show numbers');
    expect(el.textContent).not.toMatch(/bpm/);

    toggle().click();
    await whenStable();

    const text = el.textContent.replace(/\s+/g, ' ');
    expect(toggle().textContent.trim()).toBe('Hide numbers');
    expect(text).toContain('Heart rate: 66 bpm');
    expect(text).toContain('Breathing: 6 breaths/min');
  });

  it('shows a dash for a missing number', async () => {
    fake.emit('estimate', estimate({ hrBpm: null, rrRpm: 6, confidence: 'low' }));
    service.showNumbers.set(true);
    await whenStable();

    expect(el.textContent.replace(/\s+/g, ' ')).toContain('Heart rate: — bpm');
  });

  it('shows the reason instead of numbers when the confidence is invalid', async () => {
    service.showNumbers.set(true);
    fake.emit(
      'estimate',
      estimate({ hrBpm: null, rrRpm: null, breathPhase: null, confidence: 'invalid', reason: 'too_much_motion' }),
    );
    await whenStable();

    expect(el.textContent).toContain('Hold still');
    expect(el.textContent).not.toMatch(/bpm/);
  });

  it('sets the breathing circle period from the target rhythm', () => {
    const circle = el.querySelector<HTMLElement>('.breath-circle');
    expect(circle?.style.getPropertyValue('--breath-period')).toBe(`${String(60 / TARGET_RPM)}s`);
  });

  it('labels the guide and the measured phase', async () => {
    expect(el.querySelector('[data-testid="guide-label"]')?.textContent.trim()).toBe('Breathe in');
    expect(el.querySelector('[data-testid="measured-phase"]')?.textContent.trim()).toBe('You: in');

    fake.emit('estimate', estimate({ breathPhase: null }));
    await whenStable();
    expect(el.querySelector('[data-testid="measured-phase"]')).toBeNull();
  });

  describe('guideLabel', () => {
    it('inhales for 40% of the period, then exhales', () => {
      expect(guideLabel(0, 10)).toBe('Breathe in');
      expect(guideLabel(3.9, 10)).toBe('Breathe in');
      expect(guideLabel(4, 10)).toBe('Breathe out');
      expect(guideLabel(9.9, 10)).toBe('Breathe out');
      expect(guideLabel(10, 10)).toBe('Breathe in');
    });
  });
});
