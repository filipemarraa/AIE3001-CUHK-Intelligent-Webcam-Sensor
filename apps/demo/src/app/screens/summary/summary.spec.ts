import { TestBed } from '@angular/core/testing';
import { beforeEach, describe, expect, it } from 'vitest';
import { FakeVitals, estimate } from '../../testing/fake-vitals';
import { VITALS_FACTORY, VitalsService } from '../../vitals.service';
import { Summary } from './summary';

describe('Summary', () => {
  let fake: FakeVitals;
  let service: VitalsService;

  beforeEach(async () => {
    fake = new FakeVitals();
    TestBed.configureTestingModule({ providers: [{ provide: VITALS_FACTORY, useValue: () => fake }] });
    service = TestBed.inject(VitalsService);
    await service.consentAndStart(document.createElement('video'));
    fake.emit('estimate', estimate());
  });

  async function render(): Promise<{ el: HTMLElement; text: string }> {
    service.finishExercise();
    const fixture = TestBed.createComponent(Summary);
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;
    return { el, text: el.textContent.replace(/\s+/g, ' ') };
  }

  it('shows the exercise results', async () => {
    fake.summaryValue = { hrStart: 70, hrEnd: 64, rrMean: 6.2, qualityMean: 0.804, durationS: 120, method: 'pos' };
    const { text } = await render();

    expect(text).toContain('Heart rate at start: 70 bpm');
    expect(text).toContain('Heart rate at end: 64 bpm');
    expect(text).toContain('Average breathing: 6.2 breaths/min');
    expect(text).toContain('Signal quality: 80%');
  });

  it('rounds the heart rate to an integer and the breathing rate to one decimal', async () => {
    fake.summaryValue = { hrStart: 70.4, hrEnd: 63.6, rrMean: 6.0412, qualityMean: 0.8, durationS: 120, method: 'pos' };
    const { text } = await render();

    expect(text).toContain('Heart rate at start: 70 bpm');
    expect(text).toContain('Heart rate at end: 64 bpm');
    expect(text).toContain('Average breathing: 6.0 breaths/min');
  });

  it('shows a dash for missing values', async () => {
    fake.summaryValue = { hrStart: null, hrEnd: null, rrMean: null, qualityMean: null, durationS: 5, method: 'pos' };
    const { text } = await render();

    expect(text).toContain('Heart rate at start: — bpm');
    expect(text).toContain('Average breathing: — breaths/min');
    expect(text).toContain('Signal quality: —');
  });

  it('"Start again" returns to the welcome screen', async () => {
    fake.summaryValue = { hrStart: 70, hrEnd: 64, rrMean: 6, qualityMean: 0.8, durationS: 120, method: 'pos' };
    const { el } = await render();
    const button = Array.from(el.querySelectorAll('button')).find((b) => b.textContent.trim() === 'Start again');

    button?.click();

    expect(service.screen()).toBe('welcome');
    expect(service.summary()).toBeNull();
  });
});
