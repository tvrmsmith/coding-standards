using System;

namespace Fixtures;

// One declaration of every member kind the widened collector gives a span, so a kind that stops
// being collected fails under its own name.
public interface IShifted
{
    int Described { get; }

    int Describe();
}

public class Widened : IShifted
{
    private static readonly int Seed;
    private EventHandler? _handler;
    private int _count;

    static Widened()
    {
        Seed = 1;
    }

    public Widened(int count)
    {
        if (count > 0)
        {
            _count = count;
        }
    }

    ~Widened()
    {
        _count = 0;
    }

    public int Id { get; set; }

    public int Started { get; init; }

    public int Count
    {
        get
        {
            if (_count > 0)
            {
                return _count;
            }

            return 0;
        }
        set => _count = value;
    }

    public int Total => _count + Seed;

    public int this[int index] => index > 0 ? _count : 0;

    public int this[string key]
    {
        get
        {
            return key.Length;
        }

        set
        {
            _count = value;
        }
    }

    public event EventHandler? Changed
    {
        add
        {
            if (value is not null)
            {
                _handler += value;
            }
        }
        remove => _handler -= value;
    }

    int IShifted.Described => _count;

    int IShifted.Describe() => _count;

    public static Widened operator +(Widened a, Widened b) => new(a._count + b._count);

    public static Widened operator checked +(Widened a, Widened b) => new(checked(a._count + b._count));

    public static Widened operator +(Widened a) => a;

    public static Widened operator -(Widened a) => new(-a._count);

    public static Widened operator -(Widened a, Widened b) => new(a._count - b._count);

    public static implicit operator int(Widened w) => w._count;

    public static explicit operator long(Widened w) => w._count;

    public static explicit operator checked short(Widened w) => checked((short)w._count);
}
